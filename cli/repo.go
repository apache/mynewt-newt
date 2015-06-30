/*
 Copyright 2015 Stack Inc.
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
	"os"
	"path"
)

// Represents a stack repository
type Repo struct {
	// Name of the stack repository
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

// Create a new repository and initialize it
func NewRepo() (*Repo, error) {
	r := &Repo{}
	err := r.Init()

	return r, err
}

func (r *Repo) CreateCompiler(cName string) error {
	basePath := r.BasePath + "/compiler/" + cName + "/"
	cfgFile := basePath + "compiler.yml"

	if NodeExist(basePath) {
		return NewStackError("Compiler " + cName + " already exists!")
	}

	err := os.MkdirAll(basePath, 0755)
	if err != nil {
		return NewStackError(err.Error())
	}

	compilerDef := `compiler.path.cc: /path/to/compiler
compiler.path.archive: /path/to/archiver
compiler.flags.default: -default -compiler -flags
compiler.flags.debug: [compiler.flags.default, -additional -debug -flags]`

	err = ioutil.WriteFile(cfgFile, []byte(compilerDef), 0644)
	if err != nil {
		return NewStackError(err.Error())
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
		return rFile, NewStackError(err.Error())
	}

	for {
		rFile = curDir + "/repo.yml"
		if _, err := os.Stat(rFile); err == nil {
			break
		}

		curDir = path.Clean(curDir + "../../")
		if curDir == "/" {
			rFile = ""
			err = NewStackError("No repo file found!")
			break
		}
	}

	return rFile, err
}

// Create the contents of the configuration database
func (r *Repo) createDb(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS stack_cfg (
		cfg_name VARCHAR(255) NOT NULL,
		key VARCHAR(255) NOT NULL,
		value TEXT
	)
	`
	_, err := db.Exec(query)
	if err != nil {
		return NewStackError(err.Error())
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
	rows, err := db.Query("SELECT * FROM stack_cfg")
	if err != nil {
		return NewStackError(err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var cfgName sql.NullString
		var cfgKey sql.NullString
		var cfgVal sql.NullString
		err := rows.Scan(&cfgName, &cfgKey, &cfgVal)
		if err != nil {
			return NewStackError(err.Error())
		}

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
		return "", NewStackError("No configuration section exists")
	}

	val, ok := sectMap[key]
	if !ok {
		return "", NewStackError("No configuration variable exists")
	}

	return val, nil
}

func (r *Repo) GetConfigSect(sect string) (map[string]string, error) {
	sm, ok := r.Config[sect]
	if !ok {
		return nil, NewStackError("No configuration section exists")
	}

	return sm, nil
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
	db := r.db

	tx, err := db.Begin()
	if err != nil {
		return NewStackError(err.Error())
	}

	stmt, err := tx.Prepare(
		"UPDATE stack_cfg SET value=? WHERE cfg_name=? AND key=?")
	if err != nil {
		return NewStackError(err.Error())
	}
	defer stmt.Close()

	res, err := stmt.Exec(val, sect, key)
	if err != nil {
		return NewStackError(err.Error())
	}

	// Value already existed, and we updated it.  Mission accomplished!
	// Exit
	if affected, err := res.RowsAffected(); affected > 0 && err == nil {
		tx.Commit()
		return nil
	}

	// Otherwise, insert a new row
	stmt1, err := tx.Prepare("INSERT INTO stack_cfg VALUES (?, ?, ?)")
	if err != nil {
		return NewStackError(err.Error())
	}
	defer stmt1.Close()

	_, err = stmt1.Exec(sect, key, val)
	if err != nil {
		return NewStackError(err.Error())
	}

	tx.Commit()

	return nil
}

// Load the repo configuration file
func (r *Repo) loadConfig() error {
	v, err := ReadConfig(r.BasePath, "repo")
	if err != nil {
		return NewStackError(err.Error())
	}

	r.Repo = v.GetString("repo.name")
	if r.Repo == "" {
		return NewStackError("Repo file must specify repo name")
	}

	return nil
}

func CreateRepo(rName string) (*Repo, error) {
	repo := &Repo{
		Repo: rName,
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, NewStackError(err.Error())
	}

	repo.BasePath = cwd + "/" + rName + "/"
	repo.RepoFile = repo.BasePath + "repo.yml"

	os.MkdirAll(repo.BasePath, 0755)

	repoContents := "repo.name: " + rName + "\n"
	err = ioutil.WriteFile(repo.RepoFile, []byte(repoContents), 0644)
	if err != nil {
		return nil, NewStackError(err.Error())
	}

	// make base directory structure
	os.MkdirAll(repo.BasePath+"/pkg", 0755)
	os.MkdirAll(repo.BasePath+"/project", 0755)
	os.MkdirAll(repo.BasePath+"/hw/bsp", 0755)
	os.MkdirAll(repo.BasePath+"/compiler", 0755)

	return repo, nil
}

// Initialize the repo, load configuration
func (r *Repo) Init() error {
	var err error

	if r.RepoFile, err = r.getRepoFile(); err != nil {
		return err
	}

	r.BasePath = path.Dir(r.RepoFile)

	if err = r.loadConfig(); err != nil {
		return err
	}

	// Initialize config
	r.Config = make(map[string]map[string]string)

	dbName := r.BasePath + "/.repo.db"
	err = r.initDb(dbName)
	if err != nil {
		return err
	}

	return nil
}
