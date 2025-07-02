<!--
Copyright 2025 Deutsche Telekom IT GmbH

SPDX-License-Identifier: Apache-2.0
-->

# [0.4.0](https://github.com/telekom/controlplane/compare/v0.3.0...v0.4.0) (2025-06-11)


### Features

* **installation:** added installation script and instructions; smaller code-adjustments to support installation ([6b54c63](https://github.com/telekom/controlplane/commit/6b54c63686df9e8450d6b7e749761c6166ec99de))

# [0.3.0](https://github.com/telekom/controlplane/compare/v0.2.1...v0.3.0) (2025-05-28)


### Bug Fixes

* **deps:** bump github.com/gofiber/fiber/v2 from 2.52.6 to 2.52.7 ([#27](https://github.com/telekom/controlplane/issues/27)) ([2a0696e](https://github.com/telekom/controlplane/commit/2a0696e159836606c22828c73c03922ea7894532))
* include the approval api in the operators go mod file ([1a0a13a](https://github.com/telekom/controlplane/commit/1a0a13a4b1a71c987e99efef22c5bf7098e3118a))
* **kubebuilder:** correct group-names; correct paths and repos; rover-deps ([d9a19ef](https://github.com/telekom/controlplane/commit/d9a19ef95bb203417d3f209bf3861a1f3990c244))
* run go mod tidy ([c6f865e](https://github.com/telekom/controlplane/commit/c6f865e03de7258947ccb2205a522445f844b581))
* temporary fix for tests until common testutils are fixed ([9edf075](https://github.com/telekom/controlplane/commit/9edf0751bd7039c49fb98fcbc93d3690590e9f5f))


### Features

* add admin domain (api and config pkg) ([61fb9b9](https://github.com/telekom/controlplane/commit/61fb9b99441d3cdabf2ab616e4356cd9abf2b99e))
* add api submodule in organization ([afab5d1](https://github.com/telekom/controlplane/commit/afab5d1b89bcdcc2c413d942d35c06e6288f174e))
* add approval domain ([9d089cd](https://github.com/telekom/controlplane/commit/9d089cd08eb2b33e422de821a9dffb66bc4b49b2))
* add go.sum.license for go.mod in admin domain ([ef30ffc](https://github.com/telekom/controlplane/commit/ef30ffcbf04cd608295bdc8fd033feaaa5b6601e))
* add identity domain ([#23](https://github.com/telekom/controlplane/issues/23)) ([3bd1207](https://github.com/telekom/controlplane/commit/3bd1207d892ca416e55034cddc94f335319bc948))
* add organization domain ([0f78bfe](https://github.com/telekom/controlplane/commit/0f78bfe9aaa14fa977b1ef07a58b37bae2d39886))
* add organization domain in goreleaser for kos ([63f7273](https://github.com/telekom/controlplane/commit/63f72734f849fa3cb9f3312244c623539ee4de0a))
* added gateway module ([#30](https://github.com/telekom/controlplane/issues/30)) ([5c1a643](https://github.com/telekom/controlplane/commit/5c1a643d77bdb59ca4aea585e8873867c4ac15fb))
* adjust go.mod and paths after rebase ([cc325b6](https://github.com/telekom/controlplane/commit/cc325b64dbd8022e4e8d0828c463f2924a8d391f))
* **admin:** WIP add admin operator ([3ff51d7](https://github.com/telekom/controlplane/commit/3ff51d7dd2a222df046c72e19a657d55db143f9d))
* **api:** WIP add api operator ([421c5d3](https://github.com/telekom/controlplane/commit/421c5d334760936e8c066c0921c105d56149f8bd))
* **application:** WIP add application operator ([5e0cb32](https://github.com/telekom/controlplane/commit/5e0cb320c1b8b48fbd0682981b04d226964deba9))
* **rover:** add integration with the secret manager ([c3b4c20](https://github.com/telekom/controlplane/commit/c3b4c200a137243f5d4eac8f7320ee8ed39cb36a))
* **rover:** fix import path for secret manager (mistake) ([3337b83](https://github.com/telekom/controlplane/commit/3337b838cdb8299ae92bfc328a03cd7061534a98))
* **rover:** WIP add rover operator ([30debe3](https://github.com/telekom/controlplane/commit/30debe3ec1a3cb7ae118b9b59a3ca7ffc2e6d665))

## [0.2.1](https://github.com/telekom/controlplane/compare/v0.2.0...v0.2.1) (2025-05-20)


### Bug Fixes

* **goreleaser:** correct base-image; added opencontainers labels ([#14](https://github.com/telekom/controlplane/issues/14)) ([80fdae9](https://github.com/telekom/controlplane/commit/80fdae952d76e2cddc20d72e7a742274d79b4684))

# [0.2.0](https://github.com/telekom/controlplane/compare/v0.1.0...v0.2.0) (2025-05-19)


### Bug Fixes

* **reuse:** added license headers to common-server/pkg ([4a3d611](https://github.com/telekom/controlplane/commit/4a3d611093b1990eed387681d4a65edade5897be))


### Features

* added shared modules for common, common-server and secret-manager ([#7](https://github.com/telekom/controlplane/issues/7)) ([6af3eae](https://github.com/telekom/controlplane/commit/6af3eae7cb3eb2e03fd850e7246664429cefee70))
