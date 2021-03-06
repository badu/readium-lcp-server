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

package model

import (
	"strconv"
)

type (
	ContentCollection []*Content
	Content           struct {
		Id            string `json:"id" gorm:"primary_key;size:36"` // uuid - max size 36
		EncryptionKey []byte `json:"-" gorm:"size:64"`              // max size 64 (size is ignored because type is blob)
		Location      string `json:"location" gorm:"type:text"`     // text
		Length        int64  `json:"length"`                        // not exported in license spec?
		Sha256        string `json:"sha256" gorm:"size:64"`         // max size 64 - not exported in license spec?
	}
)

// Implementation of Stringer
func (c ContentCollection) GoString() string {
	result := ""
	for _, e := range c {
		result += e.GoString() + "\n"
	}
	return result
}

// Implementation of Stringer
func (c Content) GoString() string {
	return "ID : " + c.Id + " Location : " + c.Location + " Length : " + strconv.Itoa(int(c.Length))
}

func (c Content) Validate() error {
	println("Validate called.")
	return nil
}

// Implementation of GORM callback
func (c *Content) BeforeSave() error {
	if c.Id == "" {
		// Create uuid
		uid, errU := NewUUID()
		if errU != nil {
			return errU
		}
		c.Id = uid.String()
	}
	return nil
}

// Implementation of GORM Tabler
func (c *Content) TableName() string {
	return LCPContentTableName
}

func (s contentStore) Get(id string) (*Content, error) {
	var result Content
	return &result, s.db.Where(Content{Id: id}).Find(&result).Error
}

func (s contentStore) Add(newContent *Content) error {
	return s.db.Create(newContent).Error
}

func (s contentStore) Update(updatedContent *Content) error {
	return s.db.Save(updatedContent).Error
}

func (s contentStore) UpdateTitle(c *Content) error {
	return s.db.Model(c).Updates(map[string]interface{}{
		"location": c.Location,
	}).Error
}

// TODO : shouldn't we have pagination here as well ?
func (s contentStore) List() (ContentCollection, error) {
	var result ContentCollection
	return result, s.db.Find(&result).Error
}

func (s contentStore) Delete(contentID string) error {
	return Transaction(s.db, func(tx txStore) error {
		err := tx.Where("content_fk = ?", contentID).Delete(License{}).Error
		if err != nil {
			return err
		}
		return tx.Where("id = ?", contentID).Delete(Content{}).Error
	})
}
