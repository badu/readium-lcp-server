/*
 * Copyright (c) 2016-2018 Readium Foundation
 *
 * Redistribution and use in source and binary forms, with or without modification,
 * are permitted provided that the following conditions are met:
 *
 *  1. Redistributions of source code must retain the above copyright notice, this
 *     list of conditions and the following disclaimer.
 *  2. Redistributions in binary form must reproduce the above copyright notice,
 *     this list of conditions and the following disclaimer in the documentation and/or
 *     other materials provided with the distribution.
 *  3. Neither the name of the organization nor the names of its contributors may be
 *     used to endorse or promote products derived from this software without specific
 *     prior written permission
 *
 *  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
 *  ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 *  WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 *  DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT OWNER OR CONTRIBUTORS BE LIABLE FOR
 *  ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 *  (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
 *  LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
 *  ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 *  (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
 *  SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

package lcpserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/readium/readium-lcp-server/lib/filestor"
	"github.com/readium/readium-lcp-server/lib/http"
	"github.com/readium/readium-lcp-server/lib/logger"
	"github.com/readium/readium-lcp-server/lib/pack"
	"github.com/readium/readium-lcp-server/model"
	"github.com/satori/go.uuid"
	"gopkg.in/yaml.v1"
	"io/ioutil"
	goHttp "net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

var workingDir string

// prepare test server
func TestMain(m *testing.M) {
	var err error
	logz := logger.New()
	// working dir
	workingDir, err = os.Getwd()
	if err != nil {
		panic("Working dir error : " + err.Error())
	}
	workingDir = strings.Replace(workingDir, "\\src\\github.com\\readium\\readium-lcp-server\\controller\\lcpserver", "", -1)

	yamlFile, err := ioutil.ReadFile(workingDir + "\\config.yaml")
	if err != nil {
		panic(err)
	}
	var cfg http.Configuration
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		panic(err)
	}

	stor, err := model.SetupDB("sqlite3://file:"+workingDir+"\\lcp.sqlite?cache=shared&mode=rwc", logz, false)
	if err != nil {
		panic("Error setting up the database : " + err.Error())
	}
	err = stor.AutomigrateForLCP()
	if err != nil {
		panic("Error migrating database : " + err.Error())
	}

	certFile := cfg.Certificate.Cert
	if certFile == "" {
		panic("Must specify a certificate")
	}
	privKeyFile := cfg.Certificate.PrivateKey
	if privKeyFile == "" {
		panic("Must specify a private key")
	}

	cert, err := tls.LoadX509KeyPair(certFile, privKeyFile)
	if err != nil {
		panic(err)
	}

	storagePath := cfg.Storage.FileSystem.Directory
	if storagePath == "" {
		storagePath = workingDir + "\\files"
	}

	authFile := cfg.LcpServer.AuthFile
	if authFile == "" {
		panic("Must have passwords file")
	}

	var s3Storage filestor.Store
	if mode := cfg.Storage.Mode; mode == "s3" {
		s3Conf := filestor.S3Config{
			ID:             cfg.Storage.AccessId,
			Secret:         cfg.Storage.Secret,
			Token:          cfg.Storage.Token,
			Endpoint:       cfg.Storage.Endpoint,
			Bucket:         cfg.Storage.Bucket,
			Region:         cfg.Storage.Region,
			DisableSSL:     cfg.Storage.DisableSSL,
			ForcePathStyle: cfg.Storage.PathStyle,
		}
		s3Storage, _ = filestor.S3(s3Conf)
	} else {
		os.MkdirAll(storagePath, os.ModePerm) //ignore the error, the folder can already exist
		s3Storage = filestor.NewFileSystem(storagePath, cfg.LcpServer.PublicBaseUrl+"/files")
	}
	packager := pack.NewPackager(s3Storage, stor.Content(), 4)
	_, err = os.Stat(authFile)
	if err != nil {
		panic(err)
	}

	muxer := mux.NewRouter()
	server := &http.Server{
		Server: goHttp.Server{
			Handler:        muxer,
			Addr:           ":" + strconv.Itoa(cfg.LcpServer.Port),
			WriteTimeout:   15 * time.Second,
			ReadTimeout:    15 * time.Second,
			MaxHeaderBytes: 1 << 20,
		},
		Log:      logz,
		Cfg:      cfg,
		Readonly: cfg.LcpServer.ReadOnly,
		St:       &s3Storage,
		Model:    stor,
		Cert:     &cert,
		Src:      pack.ManualSource{},
	}

	server.InitAuth("Readium License Content Protection Server") // creates authority checker
	// CreateDefaultLinks inits the global var DefaultLinks from config data
	// ... DefaultLinks used in several places.
	model.DefaultLinks = make(map[string]string)
	for key := range cfg.License.Links {
		model.DefaultLinks[key] = cfg.License.Links[key]
	}
	logz.Printf("License server running on port %d [Readonly %t]", cfg.LcpServer.Port, cfg.LcpServer.ReadOnly)
	// Route.PathPrefix: http://www.gorillatoolkit.org/pkg/mux#Route.PathPrefix
	// Route.Subrouter: http://www.gorillatoolkit.org/pkg/mux#Route.Subrouter
	// Router.StrictSlash: http://www.gorillatoolkit.org/pkg/mux#Router.StrictSlash

	RegisterRoutes(muxer, server)

	server.Src.Feed(packager.Incoming)

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := server.ListenAndServe(); err != nil {
			logz.Printf("Error " + err.Error())
		}
	}()

	m.Run()

	wait := time.Second * 15 // the duration for which the server gracefully wait for existing connections to finish
	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	server.Shutdown(ctx)
	// Optionally, you could run srv.Shutdown in a goroutine and block on
	// <-ctx.Done() if your application should wait for other services
	// to finalize based on context cancellation.
	logz.Printf("server is shutting down.")
	os.Exit(0)
}

func TestAddContent(t *testing.T) {
	var buf bytes.Buffer

	// generate a new uuid; this will be the content id in the lcp server
	uid, errU := uuid.NewV4()
	if errU != nil {
		t.Fatalf("Error generating UUID : %v", errU)
	}
	outputPath := workingDir + "\\files\\sample.epub"

	workingDir2, err := os.Getwd()
	if err != nil {
		t.Fatalf("Working dir 2 error : " + err.Error())
	}
	workingDir2 = strings.Replace(workingDir2, "\\src\\github.com\\readium\\readium-lcp-server", "", -1)
	inputPath := workingDir2 + "\\test\\samples\\sample.epub"
	// encrypt the master file found at inputPath, write in the temp file, in the "encrypted repository"
	encryptedEpub, err := pack.CreateEncryptedEpub(inputPath, outputPath)

	contentDisposition := "SampleContentDisposition"

	payload := http.LcpPublication{
		ContentId:          uid.String(),
		ContentKey:         encryptedEpub.EncryptionKey,
		Output:             outputPath,
		ContentDisposition: &contentDisposition,
		Checksum:           &encryptedEpub.Checksum,
		Size:               &encryptedEpub.Size,
	}
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(payload)

	req, err := http.NewRequest("PUT", "http://localhost:8081/contents/"+uid.String(), bytes.NewReader(buf.Bytes())) //Create request with JSON body

	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("badu:hello")))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// making request
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	// If we got an error, and the context has been canceled, the context's error is probably more useful.
	if err != nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
	}

	if err != nil {
		t.Errorf("Error : %v", err)
		return
	}

	// we have a body, defering close
	defer resp.Body.Close()
	// reading body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body error : %v", err)
	}

	t.Logf("response : %v [http-status:%d]", string(body), resp.StatusCode)
}

func listContent() (model.ContentCollection, error) {
	req, err := http.NewRequest("GET", "http://localhost:8081/contents", nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// making request
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	// If we got an error, and the context has been canceled, the context's error is probably more useful.
	if err != nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
	}

	if err != nil {
		return nil, err
	}

	// we have a body, defering close
	defer resp.Body.Close()
	// reading body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var list model.ContentCollection

	err = json.Unmarshal(body, &list)
	if err != nil {
		return nil, err
	}
	return list, nil
}

func TestListContents(t *testing.T) {
	list, err := listContent()
	if err != nil {
		t.Errorf("Error : %v", err)
	}

	t.Logf("response :\n %#v", list)
}

func TestGetContent(t *testing.T) {
	list, err := listContent()
	if err != nil {
		t.Errorf("Error : %v", err)
	}
	if len(list) == 0 {
		t.Skipf("You don't have any contents to perform this test.")
	}

	req, err := http.NewRequest("GET", "http://localhost:8081/contents/"+list[0].Id, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// making request
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	// If we got an error, and the context has been canceled, the context's error is probably more useful.
	if err != nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
	}

	if err != nil {
		t.Errorf("Error : %v", err)
		return
	}

	// we have a body, defering close
	defer resp.Body.Close()
	// reading body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body error : %v", err)
	}

	var content model.Content

	err = json.Unmarshal(body, &content)
	if err != nil {
		t.Fatalf("Error Unmarshaling : %v", err)
	}

	t.Logf("response [http-status:%d]:\n %#v", resp.StatusCode, content)
}

func TestStoreContent(t *testing.T) {
	// TODO : implement
}

func TestListLicenses(t *testing.T) {
	req, err := http.NewRequest("GET", "http://localhost:8081/licenses?page=22&per_page=3", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("badu:hello")))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// making request
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	// If we got an error, and the context has been canceled, the context's error is probably more useful.
	if err != nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
	}

	if err != nil {
		t.Errorf("Error : %v", err)
		return
	}

	// we have a body, defering close
	defer resp.Body.Close()
	// reading body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body error : %v", err)
	}

	for hdrKey := range resp.Header {
		t.Logf("Header : %s = %s", hdrKey, resp.Header.Get(hdrKey))
	}
	t.Logf("Status code : %d", resp.StatusCode)

	if resp.StatusCode < 300 {
		var content model.LicensesCollection
		err = json.Unmarshal(body, &content)
		if err != nil {
			t.Fatalf("Error Unmarshaling : %v.\nServer response : %s", err, string(body))
		}
		t.Logf("response : %#v", content)
	} else {
		var problem http.Problem
		err = json.Unmarshal(body, &problem)
		if err != nil {
			t.Fatalf("Error Unmarshaling problem : %v.\nServer response : %s", err, string(body))
		}
		t.Logf("error response : %#v", problem)
	}
}

func TestListLicensesForContent(t *testing.T) {
	list, err := listContent()
	if err != nil {
		t.Errorf("Error : %v", err)
	}
	if len(list) == 0 {
		t.Skipf("You don't have any contents to perform this test.")
	}

	req, err := http.NewRequest("GET", "http://localhost:8081/contents/"+list[0].Id+"/licenses?page=2&per_page=1", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("badu:hello")))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// making request
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	// If we got an error, and the context has been canceled, the context's error is probably more useful.
	if err != nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
		default:
		}
	}

	if err != nil {
		t.Errorf("Error : %v", err)
		return
	}

	// we have a body, defering close
	defer resp.Body.Close()
	// reading body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body error : %v", err)
	}

	var content model.LicensesCollection

	err = json.Unmarshal(body, &content)
	if err != nil {
		t.Fatalf("Error Unmarshaling : %v", err)
	}

	t.Logf("response [http-status:%d]:\n %#v", resp.StatusCode, content)
}
