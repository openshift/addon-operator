# Welcome to Addon Operator Maintenance guide

[Overview](#overview)

[Maintenance Items](#maintenance-items)
* [Codebase](#codebase)
  * [Production Build](#production-build)
  * [Pull Requests](#pull-requests)
  * [Third-party Packages](#third-party-packages)
  * [Test Coverage](#test-coverage)
* [Bugs](#bugs)
  * [Bug Management](#bug-management)
* [Jira Tickets](#jira-tickets)
* [Service](#service)
* [Images](#images)


# Overview

This document is for documenting only the aspects of the Addon Operator that need maintenance. This is a living document and is meant to be iterated and enhanced as necessary. Furthermore, this document is necessary in order to ensure that the most important aspects of the Addon Operator service are maintained consistently by any available maintainer.


# Maintenance Items

The following are the aspects of the Addon Operator service that need maintenance. Some of them are either partially or fully maintained by automation such as a build job, or require human intervention.


## Codebase

The following are the properties of the Addon Operator codebase and its areas that need maintenance.

**Type**: Opensource

**License**: Apache-2.0

**VCS**: <https://github.com/openshift/addon-operator>

**Production Build**: <https://ci.int.devshift.net/job/openshift-addon-operator-gh-build-main/>

**Production Branch**: main


### Production Build

Changes to the Addon Operator codebase are continuously integrated and its areas that need maintenance are the following.

1. **Build Health**

   1. The Addon Operator build job must be kept healthy at all times. 
   2. Build failures must be addressed as soon as possible to avoid development or release interruptions. 
   3. The Addon Operator build job alerts on build failures by sending these alerts to the slack channel #sd-mt-sre-alert
   4. Most importantly, the Addon Operator codebase must be shippable at all times when it needs to be shipped.


### Pull Requests

Changes to the Addon Operator codebase are manually reviewed and approved by designated [reviewers and approvers](https://github.com/openshift/addon-operator/blob/main/OWNERS). Its areas that need maintenance are the following.

1. **Merge Quality**

   1. Every pull request must be reviewed and approved by a designated approver at all times before it is merged to main to help maintain the quality of the production codebase.


### Third-Party Packages

The Addon Operator codebase uses third-party packages; its areas that need maintenance are the following.

1. **Package Upgrades**

   1. Third-party packages used by the Addon Operator must be upgraded as soon as possible to primarily avoid potential problems inherited by the current state of such packages.
   2. Any security-vulnerable packages must be upgraded or mitigated as soon as possible to avoid such vulnerabilities in the Addon Operator service.


### Test Coverage

The Addon Operator codebase is continuously tested by running a set of test suites that is part of the codebase itself. Its areas that need maintenance are the following.

1. **Unit Tests**

   1. The Addon Operator codebase must at least meet an 80% code coverage.
   2. Every Addon Operator feature created must have a unit test to test it.


## Bugs

The Addon Operator codebase is not immune to bugs, hence, when these bugs are encountered they need to be managed accordingly. Its areas that need maintenance are the following.


### Bug Management

1. [****Jira****](http://issues.redhat.com)

   1. Every bug filed for the Addon Operator must be created as a Jira ticket.

   2. This Jira ticket must at least have the following fields.

      1. **Project**: MTSRE
      2. **Type**: Bug
      3. **Component/s**: addon-operator


2. **Critical Bugs**

   1. A critical bug must be resolved as soon as possible.


3. **Bug Queue**

   1. The total number of bugs maintained at any time must be at a healthy level. Therefore if it is deemed unhealthy, then a bug elimination work must be performed to reduce the total number of bugs.


## Jira Tickets

Jira tickets are created for every Addon Operator related task. These tickets may correlate to the overall state of the Addon Operator service and codebase, therefore they must be managed accordingly.


### Enhancements 

1. **New Features**

   1. New features must be reviewed with more scrutiny and by a wider group of people. This is to maintain the current stability and usability of the Addon Operator service.


2. **Refactor**

   1. Refactorization strictly must not cause changes to change the current behavior of the Addon Operator service.
   2. Refactorization that requires huge code changes must be reviewed with more scrutiny and must even be discouraged to avoid inadvertently introducing bugs.


### Ticket Queue

1. **Total number of tickets**

   1. The total number of Addon Operator tickets must be at a healthy level. This is to ensure the following:

      1. Unmanageable technical debt
      2. Overlooked bugs
      3. Overlooked maintenance items


## Service 

The Addon Operator service runs in OSD production clusters, and while automated operations are preferred over the manual ones, the reality is that there are still areas in it that need human intervention. These areas may need maintenance.


### Operability

1. **Escalation Policy**

   1. The Addon Operator service which is an OSD Operator must have a consistent and well-defined Escalation Policy that works for all the possible escalation cases.

2. **Alerts**

   1. There must be sufficient service alerts that notify Site-Reliability Engineers who are responsible for the Addon Operator service in production OSD clusters.

## Images

The following images are used by Addon operator:

* https://quay.io/repository/app-sre/addon-operator-manager?tab=tags
* https://quay.io/repository/app-sre/addon-operator-webhook?tab=tags
* https://quay.io/repository/app-sre/addon-operator-index?tab=tags
* https://quay.io/repository/app-sre/addon-operator-bundle?tab=tags *
* https://quay.io/repository/app-sre/addon-operator-package?tab=tags * 

These images should be kept free from high level vulnerabilities.  CVE's can be solved by updating the base UBI Image. An example pull request for updating the images can be found here: https://github.com/openshift/addon-operator/pull/314/files

\* Images are "FROM SCRATCH", so there's no Operating System on them and, as such, they don't support scans and will never have any CVEs.

