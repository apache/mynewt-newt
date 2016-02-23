/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package cli

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
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

	// Base path of the repo
	BasePath string

	// Store of PkgLists
	PkgLists map[string]*PkgList

	AddlPackagePaths []string

	// Configuration
	Config map[string]map[string]string

	// The database handle for the repo configuration database
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

func CreateRepo(repoName string, destDir string, tadpoleUrl string) error {
	if tadpoleUrl == "" {
		tadpoleUrl = "https://git-wip-us.apache.org/repos/asf/incubator-mynewt-tadpole.git"
	}

	if NodeExist(destDir) {
		return NewNewtError(fmt.Sprintf("Directory %s already exists, "+
			" cannot create new newt repo", destDir))
	}

	dl, err := NewDownloader()
	if err != nil {
		return err
	}

	StatusMessage(VERBOSITY_DEFAULT, "Downloading application skeleton from %s...",
		tadpoleUrl)
	if err := dl.DownloadFile(tadpoleUrl, "master", "/",
		destDir); err != nil {
		return err
	}
	StatusMessage(VERBOSITY_DEFAULT, OK_STRING)

	// Overwrite app.yml
	contents := []byte(fmt.Sprintf("app.name: %s\n", repoName))
	if err := ioutil.WriteFile(destDir+"/app.yml",
		contents, 0644); err != nil {
		return NewNewtError(err.Error())
	}

	// DONE!

	return nil
}

// Get a temporary directory to stick stuff in
func (repo *Repo) GetTmpDir(dirName string, prefix string) (string, error) {
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
// parent directories until it finds a file named app.yml
// if no repo file found in the directory heirarchy, an error is returned
func (repo *Repo) getRepoFile() (string, error) {
	rFile := ""

	curDir, err := os.Getwd()
	if err != nil {
		return rFile, NewNewtError(err.Error())
	}

	for {
		rFile = curDir + "/app.yml"
		log.Printf("[DEBUG] Searching for repo file at %s", rFile)
		if _, err := os.Stat(rFile); err == nil {
			log.Printf("[DEBUG] Found repo file at %s!", rFile)
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
func (repo *Repo) createDb(db *sql.DB) error {
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
func (repo *Repo) initDb(dbName string) error {
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return err
	}
	repo.db = db

	err = repo.createDb(db)
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

		_, ok := repo.Config[cfgName.String]
		if !ok {
			repo.Config[cfgName.String] = make(map[string]string)
		}

		repo.Config[cfgName.String][cfgKey.String] = cfgVal.String
	}

	return nil
}

// Get a configuration variable in section sect, with key
// error is populated if variable doesn't exist
func (repo *Repo) GetConfig(sect string, key string) (string, error) {
	sectMap, ok := repo.Config[sect]
	if !ok {
		return "", NewNewtError("No configuration section exists")
	}

	val, ok := sectMap[key]
	if !ok {
		return "", NewNewtError("No configuration variable exists")
	}

	return val, nil
}

func (repo *Repo) GetConfigSect(sect string) (map[string]string, error) {
	sm, ok := repo.Config[sect]
	if !ok {
		return nil, NewNewtError("No configuration section exists")
	}

	return sm, nil
}

// Delete a configuration variable in section sect with key and val
// Returns an error if configuration variable cannot be deleted
// (most likely due to database error or key not existing)
func (repo *Repo) DelConfig(sect string, key string) error {
	db := repo.db

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
func (repo *Repo) SetConfig(sect string, key string, val string) error {
	_, ok := repo.Config[sect]
	if !ok {
		repo.Config[sect] = make(map[string]string)
	}
	repo.Config[sect][key] = val

	// Store config
	log.Printf("[DEBUG] Storing value %s into key %s for section %s",
		val, sect, key)
	db := repo.db

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
func (repo *Repo) loadConfig() error {
	v, err := ReadConfig(repo.BasePath, "app")
	if err != nil {
		return NewNewtError(err.Error())
	}

	repo.Name = v.GetString("app.name")
	if repo.Name == "" {
		return NewNewtError("Application file must specify application name")
	}

	repo.AddlPackagePaths = v.GetStringSlice("app.additional_package_paths")

	return nil
}

func (repo *Repo) LoadPkgLists() error {
	files, err := ioutil.ReadDir(repo.PkgListPath)
	if err != nil {
		return err
	}
	for _, fileInfo := range files {
		file := fileInfo.Name()
		if filepath.Ext(file) == ".yml" {
			name := file[:len(filepath.Base(file))-len(".yml")]
			log.Printf("[DEBUG] Loading PkgList %s", name)
			pkgList, err := NewPkgList(repo)
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

func (repo *Repo) InitPath(repoPath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}

	if err = os.Chdir(repoPath); err != nil {
		return NewNewtError(err.Error())
	}

	log.Printf("[DEBUG] Searching for repository, starting in directory %s", cwd)

	if repo.RepoFile, err = repo.getRepoFile(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Repo file found, directory %s, loading configuration...",
		repo.RepoFile)

	repo.BasePath = filepath.ToSlash(path.Dir(repo.RepoFile))

	if err = repo.loadConfig(); err != nil {
		return err
	}

	if err = os.Chdir(cwd); err != nil {
		return NewNewtError(err.Error())
	}
	return nil
}

// Initialze the repository
// returns a NewtError on failure, and nil on success
func (repo *Repo) Init() error {
	var err error

	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}
	if err := repo.InitPath(cwd); err != nil {
		return err
	}

	log.Printf("[DEBUG] Configuration loaded!  Initializing .app database")

	// Create Repo store directory
	repo.StorePath = repo.BasePath + "/.app/"
	if NodeNotExist(repo.StorePath) {
		if err := os.MkdirAll(repo.StorePath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	// Create Repo configuration database
	repo.Config = make(map[string]map[string]string)

	dbName := repo.StorePath + "/app.db"
	if err := repo.initDb(dbName); err != nil {
		return err
	}

	log.Printf("[DEBUG] Database initialized.")

	// Load PkgLists for the current Repo
	repo.PkgListPath = repo.StorePath + "/pkg-lists/"
	if NodeNotExist(repo.PkgListPath) {
		if err := os.MkdirAll(repo.PkgListPath, 0755); err != nil {
			return NewNewtError(err.Error())
		}
	}

	repo.PkgLists = map[string]*PkgList{}

	if err := repo.LoadPkgLists(); err != nil {
		return err
	}

	return nil
}

func (repo *Repo) GetPkgLists() (map[string]*PkgList, error) {
	return repo.PkgLists, nil
}
