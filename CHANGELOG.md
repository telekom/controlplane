# [0.11.0](https://github.com/telekom/controlplane/compare/v0.10.0...v0.11.0) (2025-08-25)


### Features

* **client-metrics:** added metrics to gateway kong-client; updated common-client for options-pattern; updated secret-manager client metrics ([#137](https://github.com/telekom/controlplane/issues/137)) ([2a3250f](https://github.com/telekom/controlplane/commit/2a3250f5feab709788c87ed3446aa65c9c0d2c80))

# [0.10.0](https://github.com/telekom/controlplane/compare/v0.9.0...v0.10.0) (2025-08-20)


### Features

* add rate-limiting ([#114](https://github.com/telekom/controlplane/issues/114)) ([d3eec5b](https://github.com/telekom/controlplane/commit/d3eec5bcc580ccef2514c0d00a74580e35c85134))

# [0.9.0](https://github.com/telekom/controlplane/compare/v0.8.0...v0.9.0) (2025-08-19)


### Bug Fixes

* **admin:** correct creation of team-routes ([#118](https://github.com/telekom/controlplane/issues/118)) ([1807f4b](https://github.com/telekom/controlplane/commit/1807f4b6199df8af335205bbb745d7e0281b53b7))
* **file-manager-api:** correct token-path for client-jwt ([86cf202](https://github.com/telekom/controlplane/commit/86cf2029d26ac9342c6c9b879e83440008eee9e2))
* **gateway:** fixed bug where acl was not created; only consumers that are not being deleted are considered; removed obsolete code ([#126](https://github.com/telekom/controlplane/issues/126)) ([a722062](https://github.com/telekom/controlplane/commit/a722062aee244e74d067b6a7f67c88431bd94926))
* **organization:** register Prometheus metrics for secret manager communication ([#96](https://github.com/telekom/controlplane/issues/96)) ([ef65e15](https://github.com/telekom/controlplane/commit/ef65e15a6b2e05aea4d614734192fd4ba6ca968a))
* **secret-manager:** exit application on fatal error when init secret-manager; changed localhost default url; skip-tls-verify=true for localhost ([#125](https://github.com/telekom/controlplane/issues/125)) ([1f878ba](https://github.com/telekom/controlplane/commit/1f878ba239a37335f7d3daadbb5d4bdeb2bc9157))
* **security:** bump fiber to v2.52.9 ([#121](https://github.com/telekom/controlplane/issues/121)) ([6cd38cb](https://github.com/telekom/controlplane/commit/6cd38cb02620012097ca6edd498832315bfec436))


### Features

* add trusted teams ([#98](https://github.com/telekom/controlplane/issues/98)) ([79e08f0](https://github.com/telekom/controlplane/commit/79e08f0fe8b209d516779ecf82a8159882e252e5))
* Added Private Key JWT feature ([#95](https://github.com/telekom/controlplane/issues/95)) ([89a5698](https://github.com/telekom/controlplane/commit/89a5698eb010c548e593f180881d8c65adf9bfb1))
* added rover-server; improved rover-wehooks; improved secret-manager; fixed minor issues in other domains ([d27b183](https://github.com/telekom/controlplane/commit/d27b1839343ea267551716dcee41c5658dc94819))
* **file-manager:** Add a file-manager client and server based on the OAS ([#108](https://github.com/telekom/controlplane/issues/108)) ([b9279fd](https://github.com/telekom/controlplane/commit/b9279fd38809098c265e72426665692a804b6d10))
* **install-local:** Add installation and quickstart guide for the controlplane in a local environment ([#83](https://github.com/telekom/controlplane/issues/83)) ([4d01f17](https://github.com/telekom/controlplane/commit/4d01f17e838588dffb75bb94996321ea02998fb0))

# [0.8.0](https://github.com/telekom/controlplane/compare/v0.7.0...v0.8.0) (2025-07-22)


### Bug Fixes

* **api:** correct handling of failover edge cases;  correct handling of api-exposure default scopes ([#87](https://github.com/telekom/controlplane/issues/87)) ([859bffb](https://github.com/telekom/controlplane/commit/859bffb01aba5a2442b95b4726efb20773d2e1db))


### Features

* add unified configuration options for reconciler with spf13/viper as ENVs ([#90](https://github.com/telekom/controlplane/issues/90)) ([0d2091a](https://github.com/telekom/controlplane/commit/0d2091a2d68fa97df7ebe7ee4ff769935cbfc54d))
* added basic-auth feature ([#88](https://github.com/telekom/controlplane/issues/88)) ([5cf41c0](https://github.com/telekom/controlplane/commit/5cf41c002d854a2916cb645f963df4c9f4b38b50))
* added ip-restriction feature; refactored feature-builder to support consumer-features ([#89](https://github.com/telekom/controlplane/issues/89)) ([5a6aa2f](https://github.com/telekom/controlplane/commit/5a6aa2fa96e9f3a8be582a0ec1d1bd594a098c32))
* **remove-headers:** Added remove-headers feature ([#91](https://github.com/telekom/controlplane/issues/91)) ([4ab4b60](https://github.com/telekom/controlplane/commit/4ab4b607626e6a7125f3a4351e470ffd2907ebe3))
* **secret-manager:** add cache metrics for cache hits and misses ([#81](https://github.com/telekom/controlplane/issues/81)) ([902666f](https://github.com/telekom/controlplane/commit/902666fe0ea98f23e1ea21fc3d3d64b13cb34459))

# [0.7.0](https://github.com/telekom/controlplane/compare/v0.6.0...v0.7.0) (2025-07-14)


### Bug Fixes

* added disable-access-control; refactored custom-scopes ([4873006](https://github.com/telekom/controlplane/commit/4873006bedbf092c2c35230cb12019034ad6d116))


### Features

* added failover feature ([#77](https://github.com/telekom/controlplane/issues/77)) ([75981ef](https://github.com/telekom/controlplane/commit/75981efff4d804c06135ebec7beb34717fe686ad))
* **default-scopes:** Added provider default-scopes ([58026c6](https://github.com/telekom/controlplane/commit/58026c6321a223ee04152f873258cf100c6597b3))
* **externalIDP:** add external idp feature to api, gateway, rover ([#78](https://github.com/telekom/controlplane/issues/78)) ([6c185c4](https://github.com/telekom/controlplane/commit/6c185c43586dda48d5598796a4cbf09ff05ac2ae))
* **loadbalancing:** Add loadbalancing feature in the gateway domain using Upstreams in the RouteSpec ([eb3c625](https://github.com/telekom/controlplane/commit/eb3c625e08c9f07fdb33447a9f1d34f5f5649e95))
* **loadbalancing:** Add validation for load-balancing in rover webhook ([#82](https://github.com/telekom/controlplane/issues/82)) ([d22d198](https://github.com/telekom/controlplane/commit/d22d198900f68b2024b39ba7f2620303fc4a9636))

# [0.6.0](https://github.com/telekom/controlplane/compare/v0.5.0...v0.6.0) (2025-07-14)


### Bug Fixes

* add update_install.sh ([#70](https://github.com/telekom/controlplane/issues/70)) ([989c9fb](https://github.com/telekom/controlplane/commit/989c9fb3d351ea83133faef066a09e87bfbf9905))


### Features

* **visibility:** add Zone visibility feature ([e24a881](https://github.com/telekom/controlplane/commit/e24a8813afc43360dcb5c3657faeb5b96cf7e236))

# [0.5.0](https://github.com/telekom/controlplane/compare/v0.4.0...v0.5.0) (2025-07-03)


### Bug Fixes

* **secret-manager:** k8s jwks; bouncer for deletion; system certpool ([#58](https://github.com/telekom/controlplane/issues/58)) ([55f313b](https://github.com/telekom/controlplane/commit/55f313b2063528c702d27a1e9c0de9c42a81c71a))


### Features

* **codeql:** Use go build ./... for codeql to make sure all sources are compiled for analysis ([#63](https://github.com/telekom/controlplane/issues/63)) ([2fa5b15](https://github.com/telekom/controlplane/commit/2fa5b15167e2aced4cf9eddc315312a728f7bcde))
* **tool:** added snapshot tool ([#49](https://github.com/telekom/controlplane/issues/49)) ([019a771](https://github.com/telekom/controlplane/commit/019a771a07ca62f809e4b68cae5786b4dcb74fc9))

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
