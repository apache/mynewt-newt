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
	_ "github.com/mattn/go-sqlite3"
	"log"
	"os"
	"strings"
)

// Migrates target definitions from the legacy sqlite database to
// "targets/<target-name>/pkg.yml".  If a target is already in the new format,
// it is not modified.  On success, the legacy database is renamed to prevent
// subsequent migrations.  The database file is renamed as follows:
//     .app/app.db --> .app/legacy-app.db
//
// @return                      true if any targets were migrated.
func LegacyMigrateTargets(repo *Repo) bool {
	const TARGET_SECT_PREFIX = "_target_"

	dbName := repo.StorePath + "/app.db"
	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		return false
	}

	// Populate repo configuration
	log.Printf("[DEBUG] Reading legacy repo configuration from %s", dbName)
	rows, err := db.Query("SELECT cfg_name,key,value FROM newt_cfg")
	if err != nil {
		return false
	}
	defer rows.Close()

	// Copy all target settings from the database into the contents map.
	contents := map[string]map[string]string{}
	for rows.Next() {
		var cfgName sql.NullString
		var cfgKey sql.NullString
		var cfgVal sql.NullString

		err := rows.Scan(&cfgName, &cfgKey, &cfgVal)
		if err != nil {
			return false
		}

		if strings.HasPrefix(cfgName.String, TARGET_SECT_PREFIX) {
			mapping, ok := contents[cfgName.String]
			if !ok {
				mapping = make(map[string]string)
				contents[cfgName.String] = mapping
			}

			mapping[cfgKey.String] = cfgVal.String
		}
	}

	// Iterate through the contents map, populating the target store with each
	// target that doesn't already exist.
	anyMigrated := false
	for prefixedName, mapping := range contents {
		name := prefixedName[len(TARGET_SECT_PREFIX):len(prefixedName)]

		if !TargetExists(repo, name) {
			target := NewTarget(repo, name)
			for k, v := range mapping {
				log.Printf("[DEBUG] Migrating legacy target %s, key=%s val=%s",
					name, k, v)
				target.Vars[k] = v
			}

			err := target.Save()
			if err != nil {
				return anyMigrated
			}

			anyMigrated = true
		}
	}

	newName := repo.StorePath + "/legacy-app.db"
	log.Printf("[DEBUG] Renaming legacy database: %s --> %s", dbName, newName)
	err = os.Rename(dbName, newName)
	if err != nil {
		StatusMessage(VERBOSITY_QUIET, "Warning: Failed to rename legacy "+
			"database (%s --> %s); %s\n", dbName, newName, err.Error())
	}

	return anyMigrated
}
