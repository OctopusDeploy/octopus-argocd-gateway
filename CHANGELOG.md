# Changelog

## [2.2.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v2.1.0...v2.2.0) (2026-10-08)


### Features

* Buffer application events for 2s by default ([#290](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/290)) ([acd1a33](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/acd1a331e312cdd365ea50f9f2c57fe1dfccb93a))

## [2.1.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v2.0.1...v2.1.0) (2026-09-28)


### Features

* allow applications to inherit project annotations ([#286](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/286)) ([b7d7183](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b7d7183492b93c51f2cda5fb31a96819848d857b))

## [2.0.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v2.0.0...v2.0.1) (2026-09-17)


### Bug Fixes

* allow overriding auto-update images/registry and silence grpc health-check log noise ([#282](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/282)) ([ab423bb](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/ab423bb300934b1ad6a09e3f3325a5e47920c7d3))
* derive the leader-election lease name from the Helm release name ([#280](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/280)) ([2eecb92](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2eecb9248e246e24cfb9832052b32855eecf8001))

## [2.0.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.27.0...v2.0.0) (2026-09-08)


### ⚠ BREAKING CHANGES

The following Helm chart values have changed:
* `keepAlive.intervalSeconds` -> `healthCheck.interval` (default: `30s`)
* `keepAlive.maxConsecutiveFailures` -> `healthCheck.giveUpAfter`	(default: `5m`)

### Features

* classify Octopus errors through octopus-grpc, and budget outages by duration ([#276](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/276)) ([3ea8925](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/3ea8925f3e1097d7e9541905667a0a945be5b43a))

## [1.27.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.26.0...v1.27.0) (2026-07-24)


### Features

* Add Multi-Space Registration ([#265](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/265)) ([3a31b85](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/3a31b8529360d35ccffa3efdacaf05a72a689ecc))

## [1.26.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.25.0...v1.26.0) (2026-07-06)


### Features

* add Renovate configuration for automated dependency updates ([#229](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/229)) ([d043178](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/d0431789339b60eca2d188abc1cab969a74b6252))
* Add resync interval to application event handler ([#260](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/260)) ([7c10e31](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/7c10e31e2b3b69e96c86fc2a3ea9bd0097934ddb))
* support get application resource tree ([#219](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/219)) ([8bcdcc4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8bcdcc4e452f1f4fbe486a5af305a424e75ed81f))


### Bug Fixes

* Handle cluster permission denied errors and fix running Gateway externally ([#259](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/259)) ([b7c637f](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b7c637f5544e0dd5a25bc28491cfc3a917c4c692))

## [1.25.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.24.0...v1.25.0) (2026-06-10)


### Features

* Implement application event dispatcher with concurrent processing ([#222](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/222)) ([43b4cd6](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/43b4cd614474a71cf198a7604fb13d5617f10553))

## [1.24.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.23.0...v1.24.0) (2026-06-03)


### Features

* Support project role auth tokens ([#212](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/212)) ([2ef5caf](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2ef5cafbc34ac812368e6d03ee12a0ab0493a474))

## [1.23.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.22.0...v1.23.0) (2026-05-25)


### Features

* add pprof server configuration and validation ([#211](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/211)) ([118cc9b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/118cc9b8af5e702df27d1ea50ce3f3891cb8c5aa))
* add simulator ([#198](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/198)) ([32dd68c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/32dd68cffad491950f0f175c871295c36aa17ab3))
* Support leader election in octopus-argocd-gateway ([#214](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/214)) ([7704c01](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/7704c01b7cf705c5fd33908515d500e17acf0f07))


### Bug Fixes

* send applicaionUid in error messages ([c405244](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/c405244fd19520fa55382bbb489d5625353fd48a))

## [1.22.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.21.0...v1.22.0) (2026-05-06)


### Features

* **Dockerfile:** non-root user by default ([#204](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/204)) ([a7ed63a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/a7ed63ad462c5cdba6abd966fda2f949a50518b3))


### Bug Fixes

* outer reconnect loop instead of nested Start to stop growing stack traces in logs ([#191](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/191)) ([98db26d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/98db26d3b7824306418166f953a1051f683cc73d))

## [1.21.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.20.0...v1.21.0) (2026-04-30)


### Features

* add Prometheus metrics support ([#190](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/190)) ([ae4d273](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/ae4d27319c63fd2167f9fd9231d072d546068205))
* Introduce custom metrics ([#197](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/197)) ([981afa7](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/981afa734b1736c1b13edf9db6d95272d62d900b))

## [1.20.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.19.0...v1.20.0) (2026-04-13)


### Features

* App level keep alives ([#186](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/186)) ([5600958](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5600958012fbe7260894c55a529ea9e3cf8d4f52))

## [1.19.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.18.0...v1.19.0) (2026-03-23)


### Features

* Allow Argo CD to deploy Gateway Helm chart ([#180](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/180)) ([57b5e41](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/57b5e41a37cef5aa5b578ca1c62b753d506ae279))

## [1.18.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.17.0...v1.18.0) (2026-03-16)


### Features

* Added refresh loop to resource watcher ([#176](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/176)) ([b36cf72](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b36cf72966f315073f345b4f95f7bc06166c7ebc))

## [1.17.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.16.0...v1.17.0) (2026-03-06)


### Features

* argocd grpc-web support ([#168](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/168)) ([09b4acf](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/09b4acfd515bd618088a77e57c2e1efa7c33781e))

## [1.16.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.15.0...v1.16.0) (2026-03-05)


### Features

* Consolidate certificate props in helm chart ([#167](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/167)) ([1f85665](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1f85665a7fd0b0085de4f3c017515ef4b5903796))

## [1.15.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.14.0...v1.15.0) (2026-02-16)


### Features

* add SyncApplication implementation ([#161](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/161)) ([f50e4aa](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f50e4aa255ee1b8363a55e083b3afc6c6ac33f4a))

## [1.14.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.13.0...v1.14.0) (2026-01-29)


### Features

* separate gRPC logic from core logic ([#156](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/156)) ([d2ed94b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/d2ed94b659c8420613e92085cd56e34991411e62))
* Validate Argo CD auth token on startup ([#158](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/158)) ([5a903dc](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5a903dc6cea7c2f3202d9e975ed73c2d1863216e))

## [1.13.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.12.0...v1.13.0) (2026-01-12)


### Features

* Allow viewing previous gateway container logs ([#145](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/145)) ([345c55b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/345c55b27a0969019c4cc7d53934e092e3d698e4))

## [1.12.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.11.1...v1.12.0) (2025-12-25)


### Features

* Print server thumbprint in error msg when there's a mismatch ([#150](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/150)) ([698f4eb](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/698f4ebe5581635300b28811468a8bf25e711b91))

## [1.11.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.11.0...v1.11.1) (2025-12-18)


### Bug Fixes

* Failure to reconnect info service after stream failure ([#141](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/141)) ([d4f46a6](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/d4f46a61fa79c88e2a26c3c689e5ebc056bcd385))

## [1.11.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.10.0...v1.11.0) (2025-12-11)


### Features

* Allow info service to connect to octopus without argo cd connection ([#142](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/142)) ([7b9a075](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/7b9a075527d4a0e9ad59a53b2354a1554ce9b02b))

## [1.10.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.9.1...v1.10.0) (2025-12-05)


### Features

* allow custom certs ([#138](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/138)) ([8fbb9f4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8fbb9f4ea5e0cf80033828eacd4944188abd2ba0))

## [1.9.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.9.0...v1.9.1) (2025-12-02)


### Bug Fixes

* Prevent connection lockup and stop throwing errors when target manifest is not found ([#134](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/134)) ([e008db1](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/e008db143a79a69431d579c94bf1d0f404d895eb))

## [1.9.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.8.1...v1.9.0) (2025-11-27)


### Features

* Automatically set thumbprint in configuration ([#129](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/129)) ([da4c63a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/da4c63a5e94a2c1a320e8114b4806c0c7579fea8))

## [1.8.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.8.0...v1.8.1) (2025-11-20)


### Bug Fixes

* Fix missing port in address during connection verification ([#126](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/126)) ([8a29d30](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8a29d30dfef7e4f44c224de0a1dbc522fa6ea71b))

## [1.8.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.7.0...v1.8.0) (2025-11-19)


### Features

* Add error response to all unary responses ([#122](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/122)) ([a695fd8](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/a695fd880b6d7e0459a1483798cb19f355f9b2f0))
* Improve connection validation on startup ([#123](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/123)) ([d58e727](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/d58e727abd0e5c786cf0ca4e908d005427d11c66))

## [1.7.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.6.0...v1.7.0) (2025-11-14)


### Features

* Add new method on InfoService to get the gateway logs ([#118](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/118)) ([545056f](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/545056f9ca288df6bbf3911b4fae4a00ef046052))

## [1.6.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.5.0...v1.6.0) (2025-11-13)


### Features

* Add events service ([#115](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/115)) ([6922be7](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/6922be773432a5447aa228faaa9d84ca11023d08))
* Pod log service ([#116](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/116)) ([1050a24](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1050a24558143cf9b7f6a9c19a53a57ea133e24c))

## [1.5.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.4.0...v1.5.0) (2025-11-11)


### Features

* Resource manifests ([#112](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/112)) ([21a60b7](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/21a60b7dcca4e9ca0e761b4b10cc7d2a75891f2a))

## [1.4.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.3.0...v1.4.0) (2025-11-04)


### Features

* ArgoCD live status resource contract updates ([#108](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/108)) ([5b3dd64](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5b3dd64d11099ce6763cbc4cfc78b11a6504042b))

## [1.3.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.2.0...v1.3.0) (2025-10-23)


### Features

* Application resource watch service ([#105](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/105)) ([f990fca](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f990fca1fe0e9f822ca3150218cd1b5ad7e4bdec))

## [1.2.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.1.0...v1.2.0) (2025-10-21)


### Features

* Set max send and recv sizes to maximum ([#102](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/102)) ([a2f18f9](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/a2f18f972e337af752ac1523a03d5708a171f605))

## [1.1.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v1.0.0...v1.1.0) (2025-10-14)


### Features

* Add plaintext configuration option to disable TLS ([#99](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/99)) ([6e8f5e2](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/6e8f5e2ab7f77ac795323414d3b7adfb21f1fa93))

## [1.0.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.12.0...v1.0.0) (2025-10-01)


### Features

* Improve registration error logging  ([#93](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/93)) ([b1c74ad](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b1c74ada039fdcd1203789206a335e2a344e0043))
* Remove application list service and force set version to 1.0.0 ([#95](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/95)) ([2ea5c3e](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2ea5c3efdbf89b7bd52af9e3e77ad286ea524c00))

## [0.12.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.11.0...v0.12.0) (2025-09-23)


### Features

* Pass through web ui url through to registration api call ([#90](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/90)) ([5f373ee](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5f373eea648453734cb41c11c8cb3e09100d73a6))

## [0.11.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.10.0...v0.11.0) (2025-09-12)


### Features

* Inject Octopus prefixed cluster annotations into application annotations ([#87](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/87)) ([7b68eaa](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/7b68eaaab5500cf52ae894f84ba49b2036fb2c61))

## [0.10.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.9.0...v0.10.0) (2025-08-29)


### Features

* Prune stale applications ([#84](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/84)) ([945f846](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/945f84674bea3c5672e534e2e5cb9986a5740c0e))

## [0.9.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.8.0...v0.9.0) (2025-08-08)


### Features

* ArgoCD app event handling ([#79](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/79)) ([d100127](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/d10012780b9def7f84bddcf1c0953c500995b2f6))

## [0.8.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.7.0...v0.8.0) (2025-08-05)


### ⚠ BREAKING CHANGES

* Add resource_json to ArgoCDApplication proto ([#77](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/77))

### Features

* Add CronJob to auto update the Helm chart ([#70](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/70)) ([368640d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/368640d02a7e6f4d22505da86bb8e8d6097aeed9))
* Add resource_json to ArgoCDApplication proto ([#77](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/77)) ([ed5830d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/ed5830d75714032d077034f2db3e3acc4b05d12a))

## [0.7.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.6.0...v0.7.0) (2025-07-31)


### Features

* Add new info service ([#67](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/67)) ([4cdc48c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/4cdc48cca2ec1c1433143f619623484dedf04405))


### Bug Fixes

* add argo prefix to annotations ([#69](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/69)) ([6f83f0c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/6f83f0c8f33f5ad750c11fd935d7917d852647fe))

## [0.6.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.5.0...v0.6.0) (2025-07-18)


### Features

* Improve startup error handling and post install upgrades ([#60](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/60)) ([1d6e39e](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1d6e39e5589ab942d000fdba1026a24c79dfc0e0))

## [0.5.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.4.2...v0.5.0) (2025-07-11)


### Features

* Add support for non-secure connections to ArgoCD ([#61](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/61)) ([f2f952c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f2f952c8ec9730094866c4750805d866973ca476))

## [0.4.2](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.4.1...v0.4.2) (2025-07-02)


### Bug Fixes

* Improved stream resiliency and certificate verification logging ([#57](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/57)) ([4a70ec5](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/4a70ec5e01fa583f1606d0e1c95123f012d650ba))

## [0.4.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.4.0...v0.4.1) (2025-06-20)


### Bug Fixes

* Removed release as and updated chart.yaml to include more details ([#53](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/53)) ([74a6fd4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/74a6fd46c04f49a2824d45767e9ad03ebfab9054))

## [0.4.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.3.2...v0.4.0) (2025-06-20)


### Features

* release 0.9.0 ([#47](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/47)) ([81cf387](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/81cf387e0138609ceb8a5b6ef2013c70bcafb1a7))

## [0.3.2](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.3.1...v0.3.2) (2025-06-16)


### Bug Fixes

* Updated build workflow to only run when it's a pull request or a release for the application ([#35](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/35)) ([2274df6](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2274df62e22db793b0fc7ed8f23fb2ed16c8bcb5))

## [0.3.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.3.0...v0.3.1) (2025-06-13)


### Bug Fixes

* Image tags require `v` prefix . Also sanitize gRPC urls ([#31](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/31)) ([15af64b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/15af64bb7438c9b9fe141a6470bf13590f0a814c))

## [0.3.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.2.0...v0.3.0) (2025-06-12)


### Features

* Add stream resiliency ([#24](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/24)) ([ade2a81](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/ade2a81748c763f85f5c4f943b0978c945bd9bd9))
* Fetch and include cluster attributes in application attributes ([#22](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/22)) ([5e646d6](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5e646d6cb5a8f2d8fda8b2aae8edabc1ef636211))
* Support multiple application sources and integrate buf ([#26](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/26)) ([9c5641c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/9c5641ce3ff29fc024b57db17efc941e3ee1c513))
* Support registration via helm chart ([#25](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/25)) ([87b68d9](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/87b68d9f1d49774abf51ab49be5f21aa5fb068e5))

## [0.2.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.1.0...v0.2.0) (2025-06-04)


### Features

* Add ListApplicationService protos and connect to Octopus Server ([#18](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/18)) ([2804892](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2804892415011b005a311bde9b77dd1a9bc34660))

## [0.1.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.0.1...v0.1.0) (2025-05-21)


### Features

* Add ability to register with Octopus Server ([#17](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/17)) ([871030a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/871030a2247f7f955f504386db8e20d37d3f12c0))
* connect to ArgoCD ([#13](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/13)) ([9149a22](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/9149a22d0bd78597840d7a17d1f856406fd74ef5))

## [0.0.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/v0.0.1...v0.0.1) (2025-05-13)


### Miscellaneous Chores

* rename to argocd ([#9](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/9)) ([b5718a0](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b5718a0962725da6f88c3bd6e77cbccb631258c7))
