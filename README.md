<!--
#
# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements.  See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership.  The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License.  You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
#  KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
#
-->

# Newt

Apache Newt is a smart build and package management tool, designed for C and C++
applications in embedded contexts.  Newt was developed as a part of the
Apache Mynewt Operating System, more information on Apache Mynewt can be found
at https://mynewt.apache.org/.

# Features

Newt is a build system that can read a directory tree, build a dependency tree, and emit the right build artifacts. It then allows you to do the following:

* Download built target to board
* Generate full flash images
* Download debug images to a target board using a debugger
* Conditionally compile libraries & code based upon build settings
* Generate and download manufacturing flash images

Newt is also a source management system that allows you to do the following:

* Create reusable source distributions (called repos) from a collection of code.
* Use third-party components with licenses that are not comptatible with the ASF (Apache Software Foundation) license
* Upgrade repos


# How it Works

When Newt sees a directory tree that contains a "project.yml" file, it recognizes it as the base directory of a project, and automatically builds a package tree.
More information can be found in the "Newt Tool Manual" under Docs at https://mynewt.apache.org/.


# Getting Started

To build Apache Newt, simply run the included build.sh script.  For more
information on building and installng Apache Newt, please read
[INSTALLING](/INSTALLING.md) or the documentation on https://mynewt.apache.org/

Once you've installed newt, you can get started by creating a new project:

```no-highlight
  $ newt new your_project
```

For more information, and a tutorial for getting started, please take a look at
the [Apache Mynewt documentation](https://mynewt.apache.org/os/get_started/introduction/).



# Contributing

## Introduction

Anybody who works with Apache Mynewt can be a contributing member of the
community that develops and deploys it.  The process of releasing an operating
system for microcontrollers is never done: and we welcome your contributions
to that effort.

## Pull Requests

Apache Mynewt welcomes pull request via Github.  Discussions are done on Github,
but depending on the topic, can also be relayed to the official Apache Mynewt
developer mailing list dev@mynewt.apache.org.

If you are suggesting a new feature, please email the developer list directly,
with a description of the feature you are planning to work on.

We do not merge pull requests directly on Github, all PRs will be pulled and
pushed through https://git.apache.org/.

## Filing Bugs

Bugs can be filed on the
[Apache Mynewt Bug Tracker](https://issues.apache.org/jira/browse/MYNEWT).

Where possible, please include a self-contained reproduction case!

## Feature Requests

Feature requests should also be filed on the
[Apache Mynewt Bug Tracker](https://issues.apache.org/jira/browse/MYNEWT).
Please introduce it as a ticket type "Wish."

## Writing Tests

We love getting newt tests!  Apache Mynewt is a huge undertaking, and improving
code coverage is a win for every Apache Mynewt user.

## Writing Documentation

Contributing to documentation (in addition to writing tests), is a great way
to get involved with the Apache Mynewt project. The Newt documentation is found 
in [/docs](/docs).

## Getting Help

If you are having trouble contributing to Apache Mynewt, or just want to talk
to a human about what you're working on, you can contact us via the
[developers mailing list](mailto:dev@mynewt.apache.org).

Although not a formal channel, you can also find a number of core developers
on the #mynewt channel on Freenode.
