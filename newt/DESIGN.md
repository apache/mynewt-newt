# Newt Architecture

## Types

| Type | Description |
| ---- | ----------- |
| Project | "Everything" in the user's workspace.  This includes a set of Mynewt repos and all the packages contained therein. Defined by a `project.yml` file |
| Repo | A Mynewt repo consisting of packages.  The `newt upgrade` command downloads all of a project's repos and puts them in the `repos/` directory. Repos can depend on other repos. Defined by `repository.yml`. |
| Package | Contains source (`src/`), headers (`include/`) and configuration (`syscfg.yml`).  Packages can depend on other packages.  Defined by `pkg.yml`. |

| Package type | Precedence | Description |
| ------------ | ---------- | ----------- |
| Target | 



## Project initialization

(`project/project.yml`)

The first thing newt does, regardless of which command the user specified, is initialize the Mynewt project.  Project initialization consists of these steps:

1. Load and parse the `project.yml` file.
2. Load each repo specified in `project.yml`
3. Load each repo not specified in `project.yml`, but which is depended on by an already-loaded repo.

Note: We have not loaded each repo's packages yet.  We have only create an in-memory description of each repo.

Note: It is not an error if a specified repo does not exist on disk.  This condition implies that the user has not downloaded the repo yet and must run `newt upgrade`.

4. For every repo `r` in the in-memory list, load all of `r`'s packages.

At this point, newt has a giant list of packages.  Every package from every repo that the project touches is in the list.  This includes packages that may not get used by the newt command being executed.  For example, if the user is building a target for the simulator BSP, then none of the hardware-specific packages are needed.  Newt does not know which packages will ultimately get used, so it loads all of them.

## YAML config

After newt reads and parses a YAML file, it stores it internally as a `YCfg` tree.  This structure allows settings to be conditional on arbitrary expressions.  It is described reasonably well in the comments at the top of `ycfg/ycfg.go`.

Whenever newt reads a YAML file, it should puts the contents in a YCfg object.

## Expression Evaluation

The leaves in a YCfg tree can have a condition expression.  For example, the following YAML:

```
syscfg.vals:
    OS_MAIN_STACK_SIZE: 100

syscfg.vals.BLE_DEVICE:
    OS_MAIN_STACK_SIZE: 200

syscfg.'(MSYS_1_BLOCK_COUNT > 10)':
    OS_MAIN_STACK_SIZE: 300
```

yields the following YCfg tree:

```
                                [syscfg]
                                   |
                                 [defs]
                                   |
                      [OS_MAIN_STACK_SIZE (100)]
                     /                          \
        [BLE_DEVICE (200)]                [(MSYS_1_BLOCK_COUNT > 10) (300)]
```

Each time newt needs to look up the value of `MSYS_1_BLOCK_COUNT`, both conditional expressions need to be evaluated (`BLE_DEVICE` and `(MSYS_1_BLOCK_COUNT > 10)`).  It probably would be better if newt were to convert these expressions into an intermediate format as soon as it identified them as expressions.  For now, they are stored as strings, and newt fully evaluates them every time a lookup is performed.

An expression is evaluated with a two step process:

* Lex (`parse/lex.go`)
* Parse (`parse/parse.go`)

This process is fairly typical and hopefully clear from the code.

## Package dependency resolution

Most newt commands require a package dependency resolution step.  During this step, newt calculates the following information (among other things):

1. Complete list of packages required by the command.
2. The syscfg state that emerges when all the packages in step 1 are included.

This process is complicated because the above items are inter-related.  When a new package dependency is discovered, the new package introduces new syscfg settings and overrides.  Conversely, new syscfg settings may change the dependency graph because some dependencies are *conditional*.  For example, say package Foo has the following definition:
```
pkg.deps:
    - Bar

pkg.deps.MY_SETTING:
    - Baz
```

Initially, `MY_SETTING` is undefined, so `Foo` depends on `Bar`, but not `Baz`.  However, it turns out that `Bar` defines `MY_SETTING` with a value of 1.  So, after the `Bar` dependency is processed, `Foo`'s dependency graph changes such that it depends on *both* `Bar` and `Baz`.

### Details

(`resolve/resolve.go`)

Reslution is triggered by a call to `ResolveFull()`.  The product is a `Resolution` object.  This object contains a number of items, but we will focus on the following here:

* Package list
* Syscfg state

We start with a list of "seed packages".  These are packages that are guaranteed to be included in the build.  These packages are: app, target, compiler, bsp.  These packages are added to the working set.

Next, the following procedure is executed until completion:

WS = working set

1. Does any package in WS have a hard dependency that is missing from the WS?
    a. Yes: Add the missing dependencies to WS and repeat step 1.
    b. No: Proceed to step 2.
2. Prune any extraneous packages from WS \[\*\].
3. Recalculate the syscfg state from all packages in WS \[\*\].  Did the syscfg state change?
    a. Yes: Goto step 1.
    b. No: We're done.

\[\*\]: More details below.

A "hard dependency" is a dependency expressed in a package's `pkg.deps` sequence.  The dependency may be conditional on syscfg settings.  This is different from an "API dependency", i.e., a dependency on any package that supplies a given API.  A hard dependency is "hard" because it expresses the precise package to pull into the build.

At this point, we have a complete package list and the final syscfg state.  With these two items in hand, there are a few additional things to calculate:

1. Attach every API dependency to a package supplying the API.  This is just a matter of scanning the package list for required APIs and API suppliers.
2. Some data structures required later during code generation: logcfg, sysinit, sysdown.
3. List of custom commands to run during a build (https://github.com/apache/mynewt-newt/pull/335).

Now we have a `Resolution` object!

#### Package pruning

The above procedure contains a step for pruning extra packages from the working set (step 2).  There are two types of packages that can be pruned during this step:

Orphans: https://github.com/apache/mynewt-newt/issues/233
Imposters: https://github.com/apache/mynewt-newt/pull/261

Both of these package types are pruned each time step 2 is executed.

#### Syscfg calculation

1. Start with an empty syscfg state.
2. For each package `p` in WS, add all of `p`'s `syscfg.defs` to the state.
3. For each package `p` in WS, in order of package priority \[\*\]:
    a. For each of `p`'s syscfg overrides (`syscfg.vals`), replace the old value with the override.

##### Package priorities:

Only higher priority packages can override settings defined by lower priority packages.

| Package Type | Priority |
| ------------ | -------- |
| target | 6 |
| app | 5 |
| unittest | 4 |
| bsp | 3 |
| lib | 2 |
| compiler | 1 |

## Building

This section describes what happens when newt builds a target.

First, some terminology:

| Type | Description |
| ---- | ----------- |
| Builder | A struct that compiles and links the target being built. |
| TargetBuilder | A wrapper around the builder that does some prep work before the builder can do its job. |

If the description of `TargetBuilder` seems odd, that's probably because it was created for a purpose that is now largely obsolete: split images.  The `TargetBuilder` type remains, but its purpose is somewhat vague.

Since split images are mostly unsupported, this document ignores the feature.

### High level process

The build process looks like this:

1. Create a `TargetBuilder` and `Builder`.
2. `TargetBuilder` executes all the pre-build custom commands associated with the target (see: https://github.com/apache/mynewt-newt/pull/335).
3. `Builder` builds the target (more info below).
4. `TargetBuilder` executes the pre-link custom commands (see above PR for more information).
5. `Builder` links the target.
6. `TargetBuilder` executes the post-link custom commands (see above PR for more information).

### "Builder" step

(`builder/builder.go`)

What follows is a description of step 3 in the high level build process.  The input for this step is the `Resolution` object (described earlier).

There are three steps in the build process: compile, archive, and link.

#### Compile

1. Start with an empty jobs queue.
2. Iterate the list of package's in lexicographic order by name.  For each package `p`:
    a. Create a `Compiler` object `c` with `p`'s dedicated set of compiler flags.
    b. Iterate `p`'s source files in lexicographic order by filename.  For each source file `s`:
        i. Create a "job" object (`{ s, c }`)
        ii. Append the job to the queue of jobs
3. Start `num_jobs`\[\*\] go-routines.  Each go-routine performs the following procedure:
    a. Pull a job `j` off the jobs queue.  If the queue is empty, we are done.
    b. Execute the job (compile the file) by applying `j.c` to `j.s`.
    c. If any job fails, abort all jobs.

\[\*\]: By default, `num_jobs` is the number of (virtual CPUs).  It can be overridden with `-j`.

#### Archive

Iterate the list of package's in lexicographic order by name.  For each package `p`, create a `.a` file by applying p's compiler to `p`'s list of object files produced in the previous step.

Archiving is quick.  There is no need for multithreading during this step.

#### Linking

1. Create a new `Compiler` object `c`.  `c`'s list of LFLAGS is the union of *all* packages' LFLAGS lists.
2. Produce a list of `.a` files to be linked.  This list consists of all the `.a` files produced in the "archive" step, as well as any `.a` files contained in a package's `src` directory.
3. Using the set of linker scripts specified by the BSP package, produce `.bin`, `.elf`, and other files by applying the compiler to the `.a` files from step 2.

## Repo dependency resolution

(`install/install.go`, `deprepo/*.go`)

Repo dependency resolution only happens during the `newt upgrade` command.  During this process, newt calculates which version each repo should be upgraded to and detects conflicts.

The repo dependency feature is mostly described here: https://github.com/apache/mynewt-newt/pull/365

At a high level, the process consists of two steps:
1. Build a graph expressing repo dependencies.  Each node is a `<repo,version>` pair, where `repo` is either a Mynewt repo or the `project.yml` file.
2. Traverse the graph to produce a list of repo-version pairs that newt should upgrade to.

#### Build the repo dependency graph

(`deprepo/deprepo.go: BuildDepGraph()`)

Each node in this graph represents a `<repo,version>` pair.  If there are 10 versions of `apache-mynewt-core`, then there will be 10 `apache-mynewt-core` nodes in the graph.

1. Start by adding a root node representing `project.yml`.  This node has edges to each of the dependencies specified in the `project.yml` file.
2. For each repo `r` in the project, iterate `r`'s list of versions.  For each version `v`:
    a. Iterate `v`'s list of dependencies.  For each dependency `dep`:
        i. Add a node for `dep` if it doesn't exist.
        ii. Add an edge from `v` to `dep`.

Note: repo-version-pairs depends on other repo-version-pairs.  For example:

* apache-mynewt-core 1.7.0 depends on apache-mynewt-nimble 1.2.0
* apache-mynewt-core 1.8.0 depends on apache-mynewt-nimble 1.3.0

A specific commit of a repo (as opposed to a version) *does not have any dependencies at all*:

* apache-mynewt-core 92ca78ca89ff80d2c4831192d0ffce5f467e0dab depends on *NOTHING*

This last point explains how the following requirement is met:

> If someone depends on a particular commit of repo X, then X ceases to introduce inter-repo dependencies. These dependencies must be manually added to project.yml.

#### Traverse the repo dependency graph

(`deprepo/deprepo.go: ResolveRepoDeps()`)

During this step, newt traverses the dependency graph to produce the list of requested repo-version pairs.

Two data structures are used during this process:

* Working set of nodes (WS)
* List of visited nodes

1. Add the `project.yml` node to WS.
2. For each unvisited node `n` in WS, perform a breadth-first traversal.  For each edge `e` originating from `n`:
    a. If `e`'s destination `d` is not in the visited list, insert an entry for `d` into WS:
        i. If WS already contains an entry for the same repo as `d` but for a different version, *and* the old entry is low priority\[\*\], this is a conflict.  Record the conflict and return to step 2.
        ii. If as *ii* but the old entry is high priority\[\*\], ignore the conflict and return to step 2.
        iii. Add the entry to WS.
3. Add `n` to the visited list.
4. If any unvisited nodes remain in WS, return to step 2.

When the procedure completes, the contents of WS (minus `project.yml`) is the list of requested repo-version pairs.

\[\*\]: "priority" is a concept used to enforce the following requirement: 

> If project.yml depends on a commit of repo X, this dependency overrides any inter-repo dependencies on X.

If `project.yml` depends on a *commit* of a repo, that is a high-priority dependency.  All other dependencies are low-priority dependencies.

## Code generation of staged config (sysinit, sysdown, extcmd)

`stage/stage.go` implements code generation for staged processes.  A staged process is anything where a YAML file assigns a stage (priority) to each of several functions.  The generated code consists of a sequence of function calls ordered by stage number.  If two functions have the same stage number, they are sorted by lexicographic order (by function name).

All code generation occurs in `validateAndWriteCfg()` (`builder/targetbuild.go`).

## Sysinit and sysdown

The code for sysinit and sysdown is pretty straightforward.  The `pkg.init` and `pkg.down` YAML elements are converted to `[]stage.StageFunc` slices during package resolution.  During the code generation phase, the generic `stage/stage.go` code is used for both.

## External commands (aka build-time hooks)

(see Mynewt docs ("Build-Time Hooks") for background)

The `pkg.pre_build_cmds`, `pkg.pre_link_cmds`, and `pkg.post_link_cmds` YAML elements are converted to `[]stage.StageFunc` slices during package resolution.

All external commands are executed in `(*TargetBuilder).Build()` (`builder/targetbuild.go`).  Any build-inputs generated by pre-build commands are moved to the "user pre-build" pseudo package's source directory.  This is a package that doesn't really exist in the project; it is a transient package that newt creates for each build.
