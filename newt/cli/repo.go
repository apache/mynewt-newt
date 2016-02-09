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
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
)

type Repo struct {
	// Name of the Repo
	Name string

	// Path to the Repo Store
	StorePath string

	// Path to the Repo PkgLists
	PkgListPath string

	// Repo File
	RepoFile string

	// Base path of the nest
	BasePath string

	// Store of PkgLists
	PkgLists map[string]*PkgList

	// Configuration
	Config map[string]map[string]string

	// The database handle for the nest configuration database
	db *sql.DB
}

// Create a new Repo object and initialize it
func NewRepo() (*Repo, error) {
	n := &Repo{}

	err := n.Init()
	if err != nil {
		return nil, err
	}

	return n, nil
}

// Create a Repo object constructed out of repo in given path
func NewRepoWithDir(srcDir string) (*Repo, error) {
	n := &Repo{}

	err := n.InitPath(srcDir)
	if err != nil {
		return nil, err
	}

	return n, nil
}

func CreateRepo(nestName string, destDir string, tadpoleUrl string) error {
	if tadpoleUrl == "" {
		tadpoleUrl = "https://git-wip-us.apache.org/repos/asf/incubator-mynewt-tadpole.git"
	}

	if NodeExist(destDir) {
		return NewNewtError(fmt.Sprintf("Directory %s already exists, "+
			" cannot create new newt nest", destDir))
	}

	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading nest skeleton from %s...",
		tadpoleUrl)
	if err := dl.DownloadFile(tadpoleUrl, "master", "/",
		destDir); err != nil {
		return err
	}
	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	// Overwrite nest.yml
	contents := []byte(fmt.Sprintf("nest.name: %s\n", nestName))
	if err := ioutil.WriteFile(destDir+"/nest.yml",
		contents, 0644); err != nil {
		return NewNewtError(err.Error())
	}

	// DONE!

	return nil
}

// Get a temporary directory to stick stuff in
func (nest *Repo) GetTmpDir(dirName string, prefix string) (string, error) {
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
func (nest *Repo) getRepoFile() (string, error) {
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
func (nest *Repo) createDb(db *sql.DB) error {
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
func (nest *Repo) initDb(dbName string) error {
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
	log.Printf("[DEBUG] Populating Repo configuration from %s", dbName)

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
func (nest *Repo) GetConfig(sect string, key string) (string, error) {
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

func (nest *Repo) GetConfigSect(sect string) (map[string]string, error) {
	sm, ok := nest.Config[sect]
	if !ok {
		return nil, NewNewtError("No configuration section exists")
	}

	return sm, nil
}

// Delete a configuration variable in section sect with key and val
// Returns an error if configuration variable cannot be deleted
// (most likely due to database error or key not existing)
func (nest *Repo) DelConfig(sect string, key string) error {
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
func (nest *Repo) SetConfig(sect string, key string, val string) error {
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
func (nest *Repo) loadConfig() error {
	v, err := ReadConfig(nest.BasePath, "nest")
	if err != nil {
		return NewNewtError(err.Error())
	}

	nest.Name = v.GetString("nest.name")
	if nest.Name == "" {
		return NewNewtError("Repo file must specify nest name")
	}

	return nil
}

func (nest *Repo) LoadPkgLists() error {
	files, err := ioutil.ReadDir(nest.PkgListPath)
	if err != nil {
		return err
	}
	for _, fileInfo := range files {
		file := fileInfo.Name()
		if filepath.Ext(file) == ".yml" {
			name := file[:len(filepath.Base(file))-len(".yml")]
			log.Printf("[DEBUG] Loading PkgList %s", name)
			pkgList, err := NewPkgList(nest)
			if err != nil {
				return err
			}
			if err := pkgList.Load(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (nest *Repo) InitPath(nestPath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}

	if err = os.Chdir(nestPath); err != nil {
		return NewNewtError(err.Error())
	}

	log.Printf("[DEBUG] Searching for repository, starting in directory %s", cwd)

	if nest.RepoFile, err = nest.getRepoFile(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Repo file found, directory %s, loading configuration...",
		nest.RepoFile)

	nest.BasePath = filepath.ToSlash(path.Dir(nest.RepoFile))

	if err = nest.loadConfig(); err != nil {
		return err
	}

	if err = os.Chdir(cwd); err != nil {
		return NewNewtError(err.Error())
	}
	return nil
}

// Initialze the repository
// returns a NewtError on failure, and nil on success
func (nest *Repo) Init() error {
	var err error

	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}
	if err := nest.InitPath(cwd); err != nil {
		return err
	}

	log.Printf("[DEBUG] Configuration loaded!  Initializing .nest database")

	// Create Repo store directory
	nest.StorePath = nest.BasePath + "/.nest/"
	if NodeNotExist(nest.StorePath) {
		if err := os.MkdirAll(nest.StorePath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	// Create Repo configuration database
	nest.Config = make(map[string]map[string]string)

	dbName := nest.StorePath + "/nest.db"
	if err := nest.initDb(dbName); err != nil {
		return err
	}

	log.Printf("[DEBUG] Database initialized.")

	// Load PkgLists for the current Repo
	nest.PkgListPath = nest.StorePath + "/pkgLists/"
	if NodeNotExist(nest.PkgListPath) {
		if err := os.MkdirAll(nest.PkgListPath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	nest.PkgLists = map[string]*PkgList{}

	if err := nest.LoadPkgLists(); err != nil {
		return err
	}

	return nil
}

func (nest *Repo) GetPkgLists() (map[string]*PkgList, error) {
	return nest.PkgLists, nil
}
