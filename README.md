<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
**Table of Contents**  *generated with [DocToc](https://github.com/thlorenz/doctoc)*

- [Gostats](#gostats)
  - [Overview](#overview)
  - [Installation](#installation)
  - [Building](#building)
  - [Usage](#usage)
    - [Settings](#settings)
    - [Store](#store)
      - [StatGenerators](#statgenerators)
      - [Flushing](#flushing)
    - [Scopes](#scopes)
    - [Types of Stats](#types-of-stats)
      - [Counters](#counters)
      - [Gauges](#gauges)
      - [Timers](#timers)
    - [Stats with Tags](#stats-with-tags)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

# Gostats

## Overview

Gostats is a Golang statistics library with support for Counters, Gauges, and Timers.
For full documentation visit [gostats' go doc](https://godoc.org/github.com/lyft/gostats).

## Installation

```
go get github.com/lyft/gostats
```

## Building

```
make bootstrap && make compile-test
```

## Usage

In order to start using gostats, import it to your project with:

```Go
import "github.com/lyft/gostats"
```
