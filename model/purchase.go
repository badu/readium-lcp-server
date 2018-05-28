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
	"time"
)

// Purchase status
const (
	StatusToBeRenewed  = "to-be-renewed"
	StatusToBeReturned = "to-be-returned"
	StatusError        = "error"
	StatusOk           = "ok"
)

type (
	PurchaseCollection []*Purchase
	//Purchase struct defines a user in json and database
	//PurchaseType: BUY or LOAN
	Purchase struct {
		ID              int64        `json:"id,omitempty" sql:"AUTO_INCREMENT" gorm:"primary_key"`
		PublicationId   int64        `json:"-"`
		UserId          int64        `json:"-"`
		UUID            string       `json:"uuid" sql:"NOT NULL"`
		Type            string       `json:"type"`
		Status          string       `json:"status"`
		TransactionDate time.Time    `json:"transactionDate,omitempty" sql:"DEFAULT:current_timestamp;NOT NULL"`
		LicenseUUID     *NullString  `json:"licenseUuid,omitempty"`
		StartDate       *NullTime    `json:"startDate,omitempty"`
		EndDate         *NullTime    `json:"endDate,omitempty"`
		MaxEndDate      *NullTime    `json:"maxEndDate,omitempty"`
		Publication     *Publication `json:"publication" gorm:"foreignKey:PublicationId"`
		User            *User        `json:"user" gorm:"foreignKey:UserId"`
	}
)

// Implementation of gorm Tabler
func (p *Purchase) TableName() string {
	return LUTPurchaseTableName
}

// Implementation of GORM callback
func (p *Purchase) AfterFind() error {
	// cleanup for json to omit empty
	if p.LicenseUUID != nil && !p.LicenseUUID.Valid {
		p.LicenseUUID = nil
	}
	if p.StartDate != nil && !p.StartDate.Valid {
		p.StartDate = nil
	}
	if p.EndDate != nil && !p.EndDate.Valid {
		p.EndDate = nil
	}
	if p.MaxEndDate != nil && !p.MaxEndDate.Valid {
		p.MaxEndDate = nil
	}
	return nil
}

// Implementation of GORM callback
func (p *Purchase) BeforeSave() error {
	now := TruncatedNow()
	if p.TransactionDate.IsZero() {
		p.TransactionDate = now.Time
	}
	if p.User != nil {
		p.UserId = p.User.ID
	}
	if p.Type == LOAN && p.StartDate == nil {
		p.StartDate = now
	}
	if p.UUID == "" || p.ID == 0 {
		// Create uuid
		uid, errU := NewUUID()
		if errU != nil {
			return errU
		}
		p.UUID = uid.String()
	}
	return nil
}

// Get a purchase using its id
//
func (s purchaseStore) Get(id int64) (*Purchase, error) {
	var result *Purchase
	return result, s.db.Where("id = ?", id).Preload("User").Preload("Publication").Find(&result).Error
}

// GetByLicenseID gets a purchase by the associated license id
//
func (s purchaseStore) GetByLicenseID(licenseID string) (*Purchase, error) {
	var result *Purchase
	return result, s.db.Where("license_uuid = ?", licenseID).Preload("User").Preload("Publication").Find(&result).Error
}

// List purchases, with pagination
//
func (s purchaseStore) List(page int, pageNum int) (PurchaseCollection, error) {
	var result PurchaseCollection
	return result, s.db.Offset(pageNum * page).Limit(page).Order("transaction_date DESC").Find(&result).Error

}

// ListByUser: list the purchases of a given user, with pagination
//
func (s purchaseStore) ListByUser(userID int64, page int, pageNum int) (PurchaseCollection, error) {
	var result PurchaseCollection
	return result, s.db.Where("user_id = ?", userID).Offset(pageNum * page).Limit(page).Order("transaction_date DESC").Find(&result).Error
}

// Add a purchase
//
func (s purchaseStore) Add(p *Purchase) error {
	return s.db.Create(p).Error
}

func (s purchaseStore) Update(p *Purchase) error {
	return s.db.Save(p).Error

}
