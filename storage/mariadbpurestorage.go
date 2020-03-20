/*
 * Copyright (C) 2015-2020 Virgil Security Inc.
 *
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are
 * met:
 *
 *     (1) Redistributions of source code must retain the above copyright
 *     notice, this list of conditions and the following disclaimer.
 *
 *     (2) Redistributions in binary form must reproduce the above copyright
 *     notice, this list of conditions and the following disclaimer in
 *     the documentation and/or other materials provided with the
 *     distribution.
 *
 *     (3) Neither the name of the copyright holder nor the names of its
 *     contributors may be used to endorse or promote products derived from
 *     this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE AUTHOR ''AS IS'' AND ANY EXPRESS OR
 * IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 * DISCLAIMED. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY DIRECT,
 * INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
 * SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION)
 * HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
 * STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING
 * IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 * POSSIBILITY OF SUCH DAMAGE.
 *
 * Lead Maintainer: Virgil Security Inc. <support@virgilsecurity.com>
 */

package storage

import (
	"errors"
	"strings"

	"github.com/VirgilSecurity/virgil-purekit-go/protos"
	"github.com/golang/protobuf/proto"
	"github.com/jmoiron/sqlx"

	"github.com/VirgilSecurity/virgil-purekit-go/models"
	_ "github.com/go-sql-driver/mysql"
)

type MariaDBPureStorage struct {
	db         *sqlx.DB
	Serializer *ModelSerializer
}

func (m *MariaDBPureStorage) SetSerializer(serializer *ModelSerializer) {
	m.Serializer = serializer
}

func NewMariaDBPureStorage(url string) (*MariaDBPureStorage, error) {
	db, err := sqlx.Connect("mysql", url)
	if err != nil {
		return nil, err
	}
	return &MariaDBPureStorage{db: db}, nil
}

func (m *MariaDBPureStorage) InsertUser(record *models.UserRecord) error {
	rec, err := m.Serializer.SerializeUserRecord(record)
	if err != nil {
		return err
	}
	pb, err := proto.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`INSERT INTO virgil_users (
		user_id,
		record_version,
		protobuf) 
	VALUES (?, ?, ?);`, record.UserID, record.RecordVersion, pb)
	return err
}

func (m *MariaDBPureStorage) UpdateUser(record *models.UserRecord) error {
	rec, err := m.Serializer.SerializeUserRecord(record)
	if err != nil {
		return err
	}
	pb, err := proto.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`UPDATE virgil_users 
		SET record_version=?,
		protobuf=? 
		WHERE user_id=?;`, record.RecordVersion, pb, record.UserID)
	return err
}

func (m *MariaDBPureStorage) UpdateUsers(records []*models.UserRecord, previousRecordVersion int) error {
	tx, err := m.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, record := range records {
		rec, err := m.Serializer.SerializeUserRecord(record)
		if err != nil {
			return err
		}
		pb, err := proto.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE virgil_users 
			SET record_version=?,
			protobuf=? 
			WHERE user_id=? AND record_version=?`, record.RecordVersion, pb, record.UserID, previousRecordVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *MariaDBPureStorage) parseUser(bin []byte) (*models.UserRecord, error) {
	rec := &protos.UserRecord{}
	if err := proto.Unmarshal(bin, rec); err != nil {
		return nil, err
	}
	return m.Serializer.ParseUserRecord(rec)
}

func (m *MariaDBPureStorage) SelectUser(userId string) (*models.UserRecord, error) {
	var pb []byte
	if err := m.db.Get(&pb, `SELECT protobuf 
		FROM virgil_users 
		WHERE user_id=?`, userId); err != nil {
		return nil, err
	}
	return m.parseUser(pb)
}

func (m *MariaDBPureStorage) SelectUsers(userIds ...string) ([]*models.UserRecord, error) {

	if len(userIds) == 0 {
		return []*models.UserRecord{}, nil
	}

	ids := make([]interface{}, len(userIds))
	for i := range ids {
		ids[i] = userIds[i]
	}

	rows, err := m.db.Query(`SELECT protobuf 
		FROM virgil_users 
		WHERE user_id IN (?`+strings.Repeat(",?", len(userIds)-1)+")", ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*models.UserRecord
	for rows.Next() {
		var pb []byte
		if err = rows.Scan(&pb); err != nil {
			return nil, err
		}
		rec, err := m.parseUser(pb)
		if err != nil {
			return nil, err
		}
		res = append(res, rec)
	}
	if len(res) != len(userIds) {
		return nil, errors.New("user ids mismatch")
	}
	return res, nil
}

func (m *MariaDBPureStorage) DeleteUser(userId string, cascade bool) error {
	if !cascade {
		return errors.New("unsupported")
	}
	res, err := m.db.Exec(`DELETE FROM virgil_users WHERE user_id = ?;`, userId)
	if err != nil {
		return err
	}
	if r, err := res.RowsAffected(); err != nil {
		return err
	} else if r != 1 {
		return errors.New("user not found")
	}
	return nil
}

func (m *MariaDBPureStorage) parseCellKey(bin []byte) (*models.CellKey, error) {
	rec := &protos.CellKey{}
	if err := proto.Unmarshal(bin, rec); err != nil {
		return nil, err
	}
	return m.Serializer.ParseCellKey(rec)
}

func (m *MariaDBPureStorage) SelectCellKey(userId, dataId string) (*models.CellKey, error) {
	var pb []byte
	if err := m.db.Get(&pb, `SELECT protobuf
		FROM virgil_keys
		WHERE user_id=? AND data_id=?`, userId, dataId); err != nil {
		return nil, err
	}
	return m.parseCellKey(pb)
}

func (m *MariaDBPureStorage) InsertCellKey(key *models.CellKey) error {
	pbk, err := m.Serializer.SerializeCellKey(key)
	if err != nil {
		return err
	}
	pb, err := proto.Marshal(pbk)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`INSERT INTO virgil_keys (
		user_id,
		data_id,
		protobuf) 
		VALUES (?, ?, ?);`, key.UserID, key.DataID, pb)
	return err
}

func (m *MariaDBPureStorage) UpdateCellKey(key *models.CellKey) error {
	keypb, err := m.Serializer.SerializeCellKey(key)
	if err != nil {
		return err
	}
	pb, err := proto.Marshal(keypb)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`UPDATE virgil_keys 
		SET protobuf=? 
		WHERE user_id=? AND data_id=?;`, pb, key.UserID, key.DataID)
	return err
}

func (m *MariaDBPureStorage) DeleteCellKey(userId, dataId string) error {
	res, err := m.db.Exec(`DELETE FROM virgil_keys WHERE user_id = ? AND data_id = ?;`, userId, dataId)
	if err != nil {
		return err
	}
	if r, err := res.RowsAffected(); err != nil {
		return err
	} else if r != 1 {
		return errors.New("cellKey not found")
	}
	return nil
}

func (m *MariaDBPureStorage) InsertRole(role *models.Role) error {
	rolepb, err := m.Serializer.SerializeRole(role)
	if err != nil {
		return err
	}
	pb, err := proto.Marshal(rolepb)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`INSERT INTO virgil_roles (
		role_name,
		protobuf) 
		VALUES (?, ?);`, role.RoleName, pb)
	return err
}

func (m *MariaDBPureStorage) parseRole(bin []byte) (*models.Role, error) {
	role := &protos.Role{}
	if err := proto.Unmarshal(bin, role); err != nil {
		return nil, err
	}
	return m.Serializer.ParseRole(role)
}

func (m *MariaDBPureStorage) SelectRoles(roleNames ...string) ([]*models.Role, error) {

	if len(roleNames) == 0 {
		return []*models.Role{}, nil
	}

	names := make([]interface{}, len(roleNames))
	for i := range names {
		names[i] = roleNames[i]
	}

	rows, err := m.db.Query(`SELECT protobuf 
		FROM virgil_roles 
		WHERE role_name IN (?`+strings.Repeat(",?", len(roleNames)-1)+")", names...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*models.Role
	for rows.Next() {
		var pb []byte
		if err = rows.Scan(&pb); err != nil {
			return nil, err
		}
		rec, err := m.parseRole(pb)
		if err != nil {
			return nil, err
		}
		res = append(res, rec)
	}
	if len(res) != len(roleNames) {
		return nil, errors.New("role names mismatch")
	}
	return res, nil
}

func (m *MariaDBPureStorage) InsertRoleAssignments(assignments ...*models.RoleAssignment) error {
	tx, err := m.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ra := range assignments {

		pbasgn, err := m.Serializer.SerializeRoleAssignment(ra)
		if err != nil {
			return err
		}
		pb, err := proto.Marshal(pbasgn)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO virgil_role_assignments (
			role_name,
			user_id,
			protobuf) 
			VALUES (?, ?, ?);`, ra.RoleName, ra.UserID, pb)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *MariaDBPureStorage) parseRoleAssignment(bin []byte) (*models.RoleAssignment, error) {
	ra := &protos.RoleAssignment{}
	if err := proto.Unmarshal(bin, ra); err != nil {
		return nil, err
	}
	return m.Serializer.ParseRoleAssignment(ra)
}

func (m *MariaDBPureStorage) SelectRoleAssignments(userId string) ([]*models.RoleAssignment, error) {
	rows, err := m.db.Query(`SELECT protobuf 
		FROM virgil_role_assignments 
		WHERE user_id=?;`, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*models.RoleAssignment
	for rows.Next() {
		var pb []byte
		if err = rows.Scan(&pb); err != nil {
			return nil, err
		}
		rec, err := m.parseRoleAssignment(pb)
		if err != nil {
			return nil, err
		}
		if rec.UserID != userId {
			return nil, errors.New("user id mismatch")
		}
		res = append(res, rec)
	}
	return res, nil
}

func (m *MariaDBPureStorage) SelectRoleAssignment(roleName, userId string) (*models.RoleAssignment, error) {
	var pb []byte
	if err := m.db.Get(&pb, `SELECT protobuf 
		FROM virgil_role_assignments 
		WHERE user_id=? AND role_name=?;`, userId, roleName); err != nil {
		return nil, err
	}
	return m.parseRoleAssignment(pb)
}

func (m *MariaDBPureStorage) DeleteRoleAssignments(roleName string, userIds ...string) error {
	args := make([]interface{}, len(userIds)+1)
	args[0] = roleName
	for i := 0; i < len(userIds); i++ {
		args[i+1] = userIds[i]
	}
	res, err := m.db.Exec(`DELETE FROM virgil_role_assignments WHERE role_name=? AND user_id IN (?`+strings.Repeat(",?", len(userIds)-1)+")", args...)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != int64(len(userIds)) {
		return errors.New("userIds mismatch")
	}
	return nil
}

func (m *MariaDBPureStorage) InsertGrantKey(key *models.GrantKey) error {
	pbk, err := m.Serializer.SerializeGrantKey(key)
	if err != nil {
		return err
	}
	pb, err := proto.Marshal(pbk)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`INSERT INTO virgil_grant_keys (
			user_id,
			key_id,
			record_version,
			expiration_date,
			protobuf) 
			VALUES (?, ?, ?, ?, ?);`, key.UserID, key.KeyID, key.RecordVersion, key.ExpirationDate, pb)
	return err
}

func (m *MariaDBPureStorage) parseGrantKey(bin []byte) (*models.GrantKey, error) {
	gk := &protos.GrantKey{}
	if err := proto.Unmarshal(bin, gk); err != nil {
		return nil, err
	}
	return m.Serializer.ParseGrantKey(gk)
}

func (m *MariaDBPureStorage) SelectGrantKey(userId string, keyId []byte) (*models.GrantKey, error) {
	var pb []byte
	if err := m.db.Get(&pb, `SELECT protobuf 
		FROM virgil_grant_keys 
		WHERE user_id=? AND key_id=?;`, userId, keyId); err != nil {
		return nil, err
	}
	return m.parseGrantKey(pb)
}

func (m *MariaDBPureStorage) SelectGrantKeys(recordVersion int) ([]*models.GrantKey, error) {
	rows, err := m.db.Query(`SELECT protobuf 
		FROM virgil_grant_keys 
		WHERE record_version=? 
		LIMIT 1000;`, recordVersion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*models.GrantKey
	for rows.Next() {
		var pb []byte
		if err = rows.Scan(&pb); err != nil {
			return nil, err
		}
		rec, err := m.parseGrantKey(pb)
		if err != nil {
			return nil, err
		}
		res = append(res, rec)
	}
	return res, nil
}

func (m *MariaDBPureStorage) UpdateGrantKeys(keys ...*models.GrantKey) error {
	tx, err := m.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, grantKey := range keys {
		rec, err := m.Serializer.SerializeGrantKey(grantKey)
		if err != nil {
			return err
		}
		pb, err := proto.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE virgil_grant_keys 
		SET record_version=?,
		protobuf=? 
		WHERE key_id=? AND user_id=?;`, grantKey.RecordVersion, pb, grantKey.KeyID, grantKey.UserID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *MariaDBPureStorage) DeleteGrantKey(userId string, keyId []byte) error {
	res, err := m.db.Exec(`DELETE FROM virgil_grant_keys WHERE user_id = ? AND key_id = ?;`, userId, keyId)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("userId or keyId mismatch")
	}
	return nil
}

var createSchema = `
CREATE TABLE virgil_users (
user_id CHAR(36) NOT NULL PRIMARY KEY,
record_version INTEGER NOT NULL,
    INDEX record_version_index(record_version),
    UNIQUE INDEX user_id_record_version_index(user_id, record_version),
protobuf VARBINARY(2048) NOT NULL
);

CREATE TABLE virgil_keys (
user_id CHAR(36) NOT NULL,
    FOREIGN KEY (user_id)
        REFERENCES virgil_users(user_id)
        ON DELETE CASCADE,
data_id VARCHAR(128) NOT NULL,
protobuf VARBINARY(32768) NOT NULL, /* Up to 125 recipients */
    PRIMARY KEY(user_id, data_id)
);

CREATE TABLE virgil_roles (
role_name VARCHAR(64) NOT NULL PRIMARY KEY,
protobuf VARBINARY(256) NOT NULL
);

CREATE TABLE virgil_role_assignments (
role_name VARCHAR(64) NOT NULL,
    FOREIGN KEY (role_name)
        REFERENCES virgil_roles(role_name)
        ON DELETE CASCADE,
user_id CHAR(36) NOT NULL,
    FOREIGN KEY (user_id)
        REFERENCES virgil_users(user_id)
        ON DELETE CASCADE,
    INDEX user_id_index(user_id),
protobuf VARBINARY(1024) NOT NULL,
    PRIMARY KEY(role_name, user_id)
);

CREATE TABLE virgil_grant_keys (
record_version INTEGER NOT NULL,
    INDEX record_version_index(record_version),
user_id CHAR(36) NOT NULL,
    FOREIGN KEY (user_id)
        REFERENCES virgil_users(user_id)
        ON DELETE CASCADE,
key_id BINARY(64) NOT NULL,
expiration_date BIGINT NOT NULL,
    INDEX expiration_date_index(expiration_date),
protobuf VARBINARY(512) NOT NULL,
    PRIMARY KEY(user_id, key_id)
);

SET @@global.event_scheduler = 1;

CREATE EVENT delete_expired_grant_keys ON SCHEDULE EVERY $1 SECOND
    DO
        DELETE FROM virgil_grant_keys WHERE expiration_date < UNIX_TIMESTAMP();
`

var dropSchema = `
DROP TABLE IF EXISTS virgil_grant_keys, virgil_role_assignments, virgil_roles, virgil_keys, virgil_users;
DROP EVENT IF EXISTS delete_expired_grant_keys;
`

func (m *MariaDBPureStorage) InitDB(cleanGrantKeysIntervalSeconds int) error {
	_, err := m.db.Exec(createSchema, cleanGrantKeysIntervalSeconds)
	return err
}

func (m *MariaDBPureStorage) CleanDB() error {
	_, err := m.db.Exec(dropSchema)
	return err
}
