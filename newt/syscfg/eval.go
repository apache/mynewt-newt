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

package syscfg

import (
	"mynewt.apache.org/newt/newt/newtutil"
)

type ExprEvalCtx struct {
	cfg   *Cfg
	lpkgm map[string]struct{}
}

func (ctx *ExprEvalCtx) ExprGetValue(name string) (string, bool) {
	e, ok := ctx.cfg.Settings[name]

	if ok && e.EvalDone {
		panic("This should never happen :>")
	}

	return e.Value, ok
}

func (ctx *ExprEvalCtx) ExprGetValueChoices(name string) ([]string, bool) {
	e, ok := ctx.cfg.Settings[name]

	return e.ValidChoices, ok
}

func (ctx *ExprEvalCtx) ExprSetValue(name string, value interface{}, err error) {
	e, ok := ctx.cfg.Settings[name]
	if !ok {
		panic("This should never happen :>")
	}

	if e.EvalDone {
		panic("This should never happen :>")
	}

	e.EvalDone = true
	e.EvalValue = value
	if err != nil && len(err.Error()) > 0 {
		e.EvalError = err
		ctx.cfg.InvalidExpressions[name] = struct{}{}
	}

	ctx.cfg.Settings[name] = e
}

func (ctx *ExprEvalCtx) ExprQueryPkg(name string, pkgStr string) bool {
	e, ok := ctx.cfg.Settings[name]
	if !ok {
		panic("This should never happen :>")
	}

	repoName, pkgName, err := newtutil.ParsePackageString(pkgStr)
	if err != nil {
		return false
	}

	if len(repoName) == 0 {
		repoName = e.PackageDef.Repo().Name()
	}

	pkgName = newtutil.BuildPackageString(repoName, pkgName)
	_, ok = ctx.lpkgm[pkgName]

	return ok
}
