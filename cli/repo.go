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
)

// Represents a newt repository
type Repo struct {
	// Name of the newt repository
	Repo string

	// The File that contains the repo definition
	RepoFile string

	// The base directory of the Repo
	BasePath string

	// Configuration variables for this Repo
	Config map[string]map[string]string

	// The database handle for the configuration database
	db *sql.DB
}

var compilerDef string = `compiler.path.cc: /path/to/compiler
compiler.path.archive: /path/to/archiver
compiler.flags.default: -default -compiler -flags
compiler.flags.debug: [compiler.flags.default, -additional -debug -flags]`

// Create a new repository and initialize it
func NewRepo() (*Repo, error) {
	r := &Repo{}
	err := r.Init()

	return r, err
}

// Get a temporary directory to stick stuff in
func (r *Repo) GetTmpDir(dirName, prefix string) (string, error) {
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

// Create a compiler defintiion, including sample file that

func (r *Repo) CreateCompiler(cName string) error {
	basePath := r.BasePath + "/compiler/" + cName + "/"
	cfgFile := basePath + "compiler.yml"

	log.Printf("Creating a compiler definition in directory %s",
		basePath)

	if NodeExist(basePath) {
		return NewNewtError("Compiler " + cName + " already exists!")
	}

	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		return NewNewtError(err.Error())
	}

	err = ioutil.WriteFile(cfgFile, []byte(compilerDef), 0644)
	if err != nil {
		return NewNewtError(err.Error())
	}

	return nil
}

// Find the repo file.  Searches the current directory, and then recurses
// parent directories until it finds a file named .repo.yml
// if no repo file found in the directory heirarchy, an error is returned
func (r *Repo) getRepoFile() (string, error) {
	rFile := ""

	curDir, err := os.Getwd()
	if err != nil {
		return rFile, NewNewtError(err.Error())
	}

	for {
		rFile = curDir + "/repo.yml"
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
func (r *Repo) createDb(db *sql.DB) error {
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
func (r *Repo) initDb(dbName string) error {
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return err
	}
	r.db = db

	err = r.createDb(db)
	if err != nil {
		return err
	}

	// Populate repo configuration
	log.Printf("[DEBUG] Populating Repository configuration from %s", dbName)

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

		_, ok := r.Config[cfgName.String]
		if !ok {
			r.Config[cfgName.String] = make(map[string]string)
		}

		r.Config[cfgName.String][cfgKey.String] = cfgVal.String
	}

	return nil
}

// Get a configuration variable in section sect, with key
// error is populated if variable doesn't exist
func (r *Repo) GetConfig(sect string, key string) (string, error) {
	sectMap, ok := r.Config[sect]
	if !ok {
		return "", NewNewtError("No configuration section exists")
	}

	val, ok := sectMap[key]
	if !ok {
		return "", NewNewtError("No configuration variable exists")
	}

	return val, nil
}

func (r *Repo) GetConfigSect(sect string) (map[string]string, error) {
	sm, ok := r.Config[sect]
	if !ok {
		return nil, NewNewtError("No configuration section exists")
	}

	return sm, nil
}

// Delete a configuration variable in section sect with key and val
// Returns an error if configuration variable cannot be deleted
// (most likely due to database error or key not existing)
func (r *Repo) DelConfig(sect string, key string) error {
	db := r.db

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
func (r *Repo) SetConfig(sect string, key string, val string) error {
	_, ok := r.Config[sect]
	if !ok {
		r.Config[sect] = make(map[string]string)
	}
	r.Config[sect][key] = val

	// Store config
	log.Printf("[DEBUG] Storing value %s into key %s for section %s",
		val, sect, key)
	db := r.db

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
func (r *Repo) loadConfig() error {
	v, err := ReadConfig(r.BasePath, "repo")
	if err != nil {
		return NewNewtError(err.Error())
	}

	r.Repo = v.GetString("repo.name")
	if r.Repo == "" {
		return NewNewtError("Repo file must specify repo name")
	}

	return nil
}

// Create the repository rName in the directory specified by dir
// returns an initialized repository on success, with error = nil
// on failure, error is set to a NewtError, and the value of
// *Repo is undefined
func CreateRepo(dir string, rName string) (*Repo, error) {
	repo := &Repo{
		Repo: rName,
	}

	log.Printf("[DEBUG] Creating repository in directory %s", dir)

	repo.BasePath = dir + "/" + rName + "/"
	repo.RepoFile = repo.BasePath + "repo.yml"

	os.MkdirAll(repo.BasePath, 0755)

	repoContents := "repo.name: " + rName + "\n"
	if err := ioutil.WriteFile(repo.RepoFile, []byte(repoContents), 0644); err != nil {
		return nil, NewNewtError(err.Error())
	}

	// make base directory structure
	paths := []string{"/pkg", "/project", "/hw/bsp", "/compiler"}
	for _, path := range paths {
		if err := os.MkdirAll(repo.BasePath+path, 0755); err != nil {
			return nil, NewNewtError(err.Error())
		}
	}

	log.Printf("[DEBUG] Repository successfully created in directory %s",
		repo.BasePath)

	return repo, nil
}

// Initialze the repository
// returns a NewtError on failure, and nil on success
func (r *Repo) Init() error {
	var err error

	cwd, err := os.Getwd()
	if err != nil {
		return NewNewtError(err.Error())
	}
	log.Printf("[DEBUG] Searching for repository, starting in directory %s", cwd)

	if r.RepoFile, err = r.getRepoFile(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Repository file found, directory %s, loading configuration...", r.RepoFile)

	r.BasePath = path.Dir(r.RepoFile)

	if err = r.loadConfig(); err != nil {
		return err
	}

	log.Printf("[DEBUG] Configuration loaded!, initializing repo database")

	// Initialize config
	r.Config = make(map[string]map[string]string)

	dbName := r.BasePath + "/.repo.db"
	err = r.initDb(dbName)
	if err != nil {
		return err
	}

	log.Printf("[DEBUG] Database initialized.")

	return nil
}
