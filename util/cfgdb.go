/*
 Copyright 2015 Runtime Inc.
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package util

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"log"
)

type CfgDb struct {
	DbPath   string
	DbPrefix string
	Config   map[string]map[string]string
	db       *sql.DB
}

func NewCfgDb(prefix string, dbPath string) (*CfgDb, error) {
	c := &CfgDb{}

	if err := c.Init(prefix, dbPath); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *CfgDb) createDb(db *sql.DB, prefix string) error {
	query := `
	CREATE TABLE IF NOT EXISTS %s_cfg (
		cfg_name VARCHAR(255) NOT NULL,
		key VARCHAR(255) NOT NULL,
		value TEXT
	)
	`
	query = fmt.Sprintf(query, prefix)

	if _, err := db.Exec(query); err != nil {
		return NewNewtError(err.Error())
	}

	return nil
}

func (c *CfgDb) Init(prefix string, dbPath string) error {
	c.DbPrefix = prefix
	c.DbPath = dbPath
	c.Config = make(map[string]map[string]string)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	if err = c.createDb(db, prefix); err != nil {
		return err
	}
	c.db = db

	log.Printf("[DEBUG] Reading config from %s for %s", dbPath, prefix)

	rows, err := db.Query(fmt.Sprintf("SELECT * FROM %s_cfg", prefix))
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var cName sql.NullString
		var cKey sql.NullString
		var cVal sql.NullString

		if err := rows.Scan(&cName, &cKey, &cVal); err != nil {
			return NewNewtError(err.Error())
		}

		log.Printf("[DEBUG] Setting sect %s, key %s to val %s", cName.String,
			cKey.String, cVal.String)

		if _, ok := c.Config[cName.String]; !ok {
			c.Config[cName.String] = make(map[string]string)
		}

		c.Config[cName.String][cKey.String] = cVal.String
	}

	return nil
}

func (c *CfgDb) GetKey(sect string, key string) (string, error) {
	sMap, ok := c.Config[sect]
	if !ok {
		return "", NewNewtError("No configuration section " + sect + " exists")
	}

	val, ok := sMap[key]
	if !ok {
		return "", NewNewtError("No configuration variable " + key +
			" in sect " + sect + "exists")
	}

	return val, nil
}

func (c *CfgDb) GetSect(sect string) (map[string]string, error) {
	sMap, ok := c.Config[sect]
	if !ok {
		return nil, NewNewtError("No configuration section " + sect + "exists")
	}
	return sMap, nil
}

func (c *CfgDb) DeleteSect(sect string) error {
	log.Printf("[DEBUG] Deleting sect %s", sect)

	tx, err := c.db.Begin()
	if err != nil {
		return NewNewtError(err.Error())
	}

	stmt, err := tx.Prepare(fmt.Sprintf(
		"DELETE FROM %s_cfg WHERE cfg_name=?", c.DbPrefix))
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer stmt.Close()

	r, err := stmt.Exec(sect)
	if err != nil {
		return err
	}

	tx.Commit()

	if naffected, err := r.RowsAffected(); naffected > 0 && err == nil {
		log.Printf("[DEBUG] Successfully deleted sect %s from db %s",
			sect, c.DbPath)
	} else {
		log.Printf("[DEBUG] Sect %s not found in db %s.  Delete "+
			"successful", sect, c.DbPath)
	}

	return nil
}

func (c *CfgDb) DeleteKey(sect string, key string) error {
	log.Printf("[DEBUG] Deleting sect %s, key %s", sect, key)

	tx, err := c.db.Begin()
	if err != nil {
		return NewNewtError(err.Error())
	}

	stmt, err := tx.Prepare(fmt.Sprintf(
		"DELETE FROM %s_cfg WHERE cfg_name=? AND key=?", c.DbPrefix))
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer stmt.Close()

	r, err := stmt.Exec(sect, key)
	if err != nil {
		return err
	}

	tx.Commit()

	if naffected, err := r.RowsAffected(); naffected > 0 && err == nil {
		log.Printf("[DEBUG] Successfully deleted sect %s, key %s from db %s",
			sect, key, c.DbPath)
	} else {
		log.Printf("[DEBUG] Sect %s, key %s not found in db %s.  Delete "+
			"successful", sect, key, c.DbPath)
	}

	return nil
}

func (c *CfgDb) SetKey(sect string, key string, val string) error {
	if _, ok := c.Config[sect]; !ok {
		log.Printf("[DEBUG] Section %s doesn't exist, creating it!", sect)
		c.Config[sect] = make(map[string]string)
	}
	c.Config[sect][key] = val

	log.Printf("[DEBUG] Storing value %s in section %s, key %s",
		val, sect, key)

	tx, err := c.db.Begin()
	if err != nil {
		return NewNewtError(err.Error())
	}

	stmt, err := tx.Prepare(fmt.Sprintf(
		"UPDATE %s_cfg SET value=? WHERE cfg_name=? AND key=?", c.DbPrefix))
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer stmt.Close()

	r, err := stmt.Exec(val, sect, key)
	if err != nil {
		return NewNewtError(err.Error())
	}

	// If update succeeded, then exit out.
	if naffected, err := r.RowsAffected(); naffected > 0 && err == nil {
		tx.Commit()
		log.Printf("[DEBUG] Sect %s, key %s successfully updated to %s",
			sect, key, val)
		return nil
	}

	// Otherwise, insert a new row.
	stmt, err = tx.Prepare(fmt.Sprintf("INSERT INTO %s_cfg VALUES (?, ?, ?)",
		c.DbPrefix))
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer stmt.Close()

	if _, err = stmt.Exec(sect, key, val); err != nil {
		return NewNewtError(err.Error())
	}

	tx.Commit()

	log.Printf("[DEBUG] Section %s, key %s successfully created.  Value set"+
		"to %s", sect, key, val)

	return nil
}
