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

package cli

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
)

type Nest struct {
	// Name of the Nest
	Name string

	// Path to the Nest Store
	StorePath string

	// Path to the Nest Clutches
	ClutchPath string

	// Nest File
	NestFile string

	// Base path of the nest
	BasePath string

	// Store of Clutches
	Clutches map[string]*Clutch

	// Configuration
	Config map[string]map[string]string

	// The database handle for the nest configuration database
	db *sql.DB
}

// Create a new Nest object and initialize it
func NewNest() (*Nest, error) {
	n := &Nest{}

	err := n.Init()
	if err != nil {
		return nil, err
	}

	return n, nil
}

// Get a temporary directory to stick stuff in
func (nest *Nest) GetTmpDir(dirName string, prefix string) (string, error) {
	tmpDir := dirName
	if NodeNotExist(tmpDir) {
		if err := os.MkdirAll(tmpDir, 0700); err != nil {
			return "", err
		}
	}

	name, err := ioutil.TempDir(tmpDir, prefix)
	if err != nil {
		return "", err
	}

	return name, nil
}

// Find the repo file.  Searches the current directory, and then recurses
// parent directories until it finds a file named .repo.yml
// if no repo file found in the directory heirarchy, an error is returned
func (nest *Nest) getNestFile() (string, error) {
	rFile := ""

	curDir, err := os.Getwd()
	if err != nil {
		return rFile, NewNewtError(err.Error())
	}

	for {
		rFile = curDir + "/nest.yml"
		log.Printf("[DEBUG] Searching for nest file at %s", rFile)
		if _, err := os.Stat(rFile); err == nil {
			log.Printf("[DEBUG] Found nest file at %s!", rFile)
			break
		}

		curDir = path.Clean(curDir + "../../")
		if curDir == "/" {
			rFile = ""
			err = NewNewtError("No repo file found!")
			break
		}
	}

	return rFile, err
}

// Create the contents of the configuration database
func (nest *Nest) createDb(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS newt_cfg (
		cfg_name VARCHAR(255) NOT NULL,
		key VARCHAR(255) NOT NULL,
		value TEXT
	)
	`
	_, err := db.Exec(query)
	if err != nil {
		return NewNewtError(err.Error())
	} else {
		return nil
	}
}

// Initialize the configuration database specified by dbName.  If the database
// doesn't exist, create it.
func (nest *Nest) initDb(dbName string) error {
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return err
	}
	nest.db = db

	err = nest.createDb(db)
	if err != nil {
		return err
	}

	// Populate repo configuration
	log.Printf("[DEBUG] Populating Nest configuration from %s", dbName)

	rows, err := db.Query("SELECT * FROM newt_cfg")
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var cfgName sql.NullString
		var cfgKey sql.NullString
		var cfgVal sql.NullString

		err := rows.Scan(&cfgName, &cfgKey, &cfgVal)
		if err != nil {
			return NewNewtError(err.Error())
		}

		log.Printf("[DEBUG] Setting sect %s, key %s to val %s", cfgName.String,
			cfgKey.String, cfgVal.String)

		_, ok := nest.Config[cfgName.String]
		if !ok {
			nest.Config[cfgName.String] = make(map[string]string)
		}

		nest.Config[cfgName.String][cfgKey.String] = cfgVal.String
	}

	return nil
}

// Get a configuration variable in section sect, with key
// error is populated if variable doesn't exist
func (nest *Nest) GetConfig(sect string, key string) (string, error) {
	sectMap, ok := nest.Config[sect]
	if !ok {
		return "", NewNewtError("No configuration section exists")
	}

	val, ok := sectMap[key]
	if !ok {
		return "", NewNewtError("No configuration variable exists")
	}

	return val, nil
}

func (nest *Nest) GetConfigSect(sect string) (map[string]string, error) {
	sm, ok := nest.Config[sect]
	if !ok {
		return nil, NewNewtError("No configuration section exists")
	}

	return sm, nil
}

// Delete a configuration variable in section sect with key and val
// Returns an error if configuration variable cannot be deleted
// (most likely due to database error or key not existing)
func (nest *Nest) DelConfig(sect string, key string) error {
	db := nest.db

	log.Printf("[DEBUG] Deleting sect %s, key %s", sect, key)

	tx, err := db.Begin()
	if err != nil {
		return NewNewtError(err.Error())
	}

	stmt, err := tx.Prepare("DELETE FROM newt_cfg WHERE cfg_name=? AND key=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	res, err := stmt.Exec(sect, key)
	if err != nil {
		return err
	}

	tx.Commit()

	if affected, err := res.RowsAffected(); affected > 0 && err == nil {
		log.Printf("[DEBUG] sect %s, key %s successfully deleted from database",
			sect, key)
	} else {
		log.Printf("[DEBUG] sect %s, key %s not found, considering \"delete\" successful",
			sect, key)
	}

	return nil
}

// Set a configuration variable in section sect with key, and val
// Returns an error if configuration variable cannot be set
// (most likely not able to set it in database.)
func (nest *Nest) SetConfig(sect string, key string, val string) error {
	_, ok := nest.Config[sect]
	if !ok {
		nest.Config[sect] = make(map[string]string)
	}
	nest.Config[sect][key] = val

	// Store config
	log.Printf("[DEBUG] Storing value %s into key %s for section %s",
		val, sect, key)
	db := nest.db

	tx, err := db.Begin()
	if err != nil {
		return NewNewtError(err.Error())
	}

	stmt, err := tx.Prepare(
		"UPDATE newt_cfg SET value=? WHERE cfg_name=? AND key=?")
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer stmt.Close()

	res, err := stmt.Exec(val, sect, key)
	if err != nil {
		return NewNewtError(err.Error())
	}

	// Value already existed, and we updated it.  Mission accomplished!
	// Exit
	if affected, err := res.RowsAffected(); affected > 0 && err == nil {
		tx.Commit()
		log.Printf("[DEBUG] Key %s, sect %s successfully updated to %s", key, sect, val)
		return nil
	}

	// Otherwise, insert a new row
	stmt1, err := tx.Prepare("INSERT INTO newt_cfg VALUES (?, ?, ?)")
	if err != nil {
		return NewNewtError(err.Error())
	}
	defer stmt1.Close()

	_, err = stmt1.Exec(sect, key, val)
	if err != nil {
		return NewNewtError(err.Error())
	}

	tx.Commit()

	log.Printf("[DEBUG] Key %s, sect %s successfully create, value set to %s",
		key, sect, val)

	return nil
}

// Load the repo configuration file
func (nest *Nest) loadConfig() error {
	v, err := ReadConfig(nest.BasePath, "nest")
	if err != nil {
		return NewNewtError(err.Error())
	}

	nest.Name = v.GetString("nest.name")
	if nest.Name == "" {
		return NewNewtError("Nest file must specify nest name")
	}

	return nil
}

func (nest *Nest) LoadClutches() error {
	files, err := ioutil.ReadDir(nest.ClutchPath)
	if err != nil {
		return err
	}
	for _, fileInfo := range files {
		file := fileInfo.Name()
		if filepath.Ext(file) == ".yml" {
			name := file[:len(filepath.Base(file))-len(".yml")]
			log.Printf("[DEBUG] Loading Clutch %s", name)
			clutch, err := NewClutch(nest)
			if err != nil {
				return err
			}
			if err := clutch.Load(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Initialze the repository
// returns a NewtError on failure, and nil on success
func (nest *Nest) Init() error {
	var err error

	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}
	log.Printf("[DEBUG] Searching for repository, starting in directory %s", cwd)

	if nest.NestFile, err = nest.getNestFile(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Nest file found, directory %s, loading configuration...",
		nest.NestFile)

	nest.BasePath = path.Clean(path.Dir(nest.NestFile))

	if err = nest.loadConfig(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Configuration loaded!  Initializing .nest database")

	// Create Nest store directory
	nest.StorePath = nest.BasePath + "/.nest/"
	if NodeNotExist(nest.StorePath) {
		if err := os.MkdirAll(nest.StorePath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	// Create Nest configuration database
	nest.Config = make(map[string]map[string]string)

	dbName := nest.StorePath + "/nest.db"
	if err := nest.initDb(dbName); err != nil {
		return err
	}

	log.Printf("[DEBUG] Database initialized.")

	// Load Clutches for the current Nest
	nest.ClutchPath = nest.StorePath + "/.clutches/"
	if NodeNotExist(nest.ClutchPath) {
		if err := os.MkdirAll(nest.ClutchPath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	if err := nest.LoadClutches(); err != nil {
		return err
	}

	return nil
}
func (nest *Nest) GetClutches() (map[string]*Clutch, error) {
	return nest.Clutches, nil
}
