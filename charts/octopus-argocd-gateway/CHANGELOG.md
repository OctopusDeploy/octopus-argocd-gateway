# Changelog

## [2.3.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v2.2.0...octopus-argocd-gateway-chart-v2.3.0) (2026-10-08)


### Features

* Buffer application events for 2s by default ([#290](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/290)) ([acd1a33](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/acd1a331e312cdd365ea50f9f2c57fe1dfccb93a))
* **main:** release 2.2.0 ([#292](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/292)) ([c9ec32a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/c9ec32a7b5fb376335a67d8341b85a0d92537d07))

## [2.2.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v2.1.0...octopus-argocd-gateway-chart-v2.2.0) (2026-09-29)


### Features

* allow applications to inherit project annotations ([#286](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/286)) ([b7d7183](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b7d7183492b93c51f2cda5fb31a96819848d857b))
* **main:** release 2.1.0 ([#289](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/289)) ([050519b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/050519bfadcd87254ec300c60c54f23cc3c18fe3))

## [2.1.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v2.0.0...octopus-argocd-gateway-chart-v2.1.0) (2026-09-18)


### Features

* **main:** release 2.0.1 ([#284](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/284)) ([fbaed5d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/fbaed5d7ba0e7a0b63e220b4bbcaf643cb573ad7))


### Bug Fixes

* allow overriding auto-update images/registry and silence grpc health-check log noise ([#282](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/282)) ([ab423bb](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/ab423bb300934b1ad6a09e3f3325a5e47920c7d3))
* apply extraEnv to the registration and auto update jobs ([#281](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/281)) ([9b3ae7d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/9b3ae7dcc434f750543ad27025207a49c90164a3))

## [2.0.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.33.0...octopus-argocd-gateway-chart-v2.0.0) (2026-09-09)


### ⚠ BREAKING CHANGES

The following Helm chart values have changed:
* `keepAlive.intervalSeconds` -> `healthCheck.interval` (default: `30s`)
* `keepAlive.maxConsecutiveFailures` -> `healthCheck.giveUpAfter`	(default: `5m`)

### Features

* classify Octopus errors through octopus-grpc, and budget outages by duration ([#276](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/276)) ([3ea8925](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/3ea8925f3e1097d7e9541905667a0a945be5b43a))
* **main:** release 2.0.0 ([#278](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/278)) ([b6217e4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b6217e47f873b9f3d58d1315dbdb8b482fd68ba9))

## [1.33.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.32.0...octopus-argocd-gateway-chart-v1.33.0) (2026-07-24)


### Features

* Add Multi-Space Registration ([#265](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/265)) ([3a31b85](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/3a31b8529360d35ccffa3efdacaf05a72a689ecc))
* **main:** release 1.27.0 ([#268](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/268)) ([f1c62fe](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f1c62fe2de601afe4d8619526229fd76a0793955))

## [1.32.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.31.0...octopus-argocd-gateway-chart-v1.32.0) (2026-07-06)


### Features

* **main:** release 1.26.0 ([#227](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/227)) ([f0e84a9](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f0e84a97992ea2f94decf2ef4b30f8d8af1ea01a))

## [1.31.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.30.0...octopus-argocd-gateway-chart-v1.31.0) (2026-06-10)


### Features

* **main:** release 1.25.0 ([#225](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/225)) ([01fc62b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/01fc62b0bad930ddf6c3c12e0838290337a50318))

## [1.30.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.29.0...octopus-argocd-gateway-chart-v1.30.0) (2026-06-08)


### Features

* add support for nodeSelector, affinity, and tolerations in auto-update job ([#223](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/223)) ([5878bc5](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5878bc5679fb48c77ef73f4e05b775649764f921))

## [1.29.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.28.0...octopus-argocd-gateway-chart-v1.29.0) (2026-06-03)


### Features

* **main:** release 1.24.0 ([#221](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/221)) ([cd3df5f](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/cd3df5f106392f2e10138992997c217a6f95ce05))
* Support project role auth tokens ([#212](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/212)) ([2ef5caf](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2ef5cafbc34ac812368e6d03ee12a0ab0493a474))

## [1.28.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.27.0...octopus-argocd-gateway-chart-v1.28.0) (2026-05-27)


### Features

* update indentation for imagePullSecrets, nodeSelector, affinity, and tolerations ([#216](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/216)) ([71df793](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/71df7937a1dd7d983f031ca5e89dcefaa54ed45b))

## [1.27.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.26.2...octopus-argocd-gateway-chart-v1.27.0) (2026-05-25)


### Features

* add pprof server configuration and validation ([#211](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/211)) ([118cc9b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/118cc9b8af5e702df27d1ea50ce3f3891cb8c5aa))
* **main:** release 1.23.0 ([#207](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/207)) ([62a61bb](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/62a61bb8d9fdedd47132e455be13c05b3936d991))
* Support leader election in octopus-argocd-gateway ([#214](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/214)) ([7704c01](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/7704c01b7cf705c5fd33908515d500e17acf0f07))

## [1.26.2](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.26.1...octopus-argocd-gateway-chart-v1.26.2) (2026-05-18)


### Bug Fixes

* do not require runAsNonRoot for updater ([#208](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/208)) ([a82e44d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/a82e44d1064a84649c944a0f363c5f002a063139))

## [1.26.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.26.0...octopus-argocd-gateway-chart-v1.26.1) (2026-05-07)


### Bug Fixes

* chart resources cause templating to fail ([#203](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/203)) ([dc4704a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/dc4704add04af9d27f46f4ce6768e8cfd57beef9))

## [1.26.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.25.0...octopus-argocd-gateway-chart-v1.26.0) (2026-05-07)


### Features

* **main:** release 1.22.0 ([#202](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/202)) ([4a5bf8e](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/4a5bf8e7eae2ceaf546daa22f812b59f575fe577))
* security context to comply restricted-v2 openshift SCC by default ([#189](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/189)) ([34bf798](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/34bf798cfb3af7ad2839f027926684bb85c34909))

## [1.25.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.24.0...octopus-argocd-gateway-chart-v1.25.0) (2026-04-30)


### Features

* add Prometheus metrics support ([#190](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/190)) ([ae4d273](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/ae4d27319c63fd2167f9fd9231d072d546068205))
* Introduce custom metrics ([#197](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/197)) ([981afa7](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/981afa734b1736c1b13edf9db6d95272d62d900b))
* **main:** release 1.21.0 ([#194](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/194)) ([0b40f05](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/0b40f05483a931a3c23394dc6490766fc397deb8))

## [1.24.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.23.0...octopus-argocd-gateway-chart-v1.24.0) (2026-04-13)


### Features

* App level keep alives ([#186](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/186)) ([5600958](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5600958012fbe7260894c55a529ea9e3cf8d4f52))
* **main:** release 1.20.0 ([#187](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/187)) ([70ecab2](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/70ecab2553be8a81fd30b5f4ccc5dd5e12b5f7a3))

## [1.23.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.22.0...octopus-argocd-gateway-chart-v1.23.0) (2026-03-26)


### Features

* Allow Argo CD to deploy Gateway Helm chart ([#180](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/180)) ([57b5e41](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/57b5e41a37cef5aa5b578ca1c62b753d506ae279))
* **main:** release 1.19.0 ([#183](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/183)) ([0d9b782](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/0d9b7822a4b6df56aed9b068dfc07bba4710c1a0))

## [1.22.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.21.0...octopus-argocd-gateway-chart-v1.22.0) (2026-03-16)


### Features

* **main:** release 1.18.0 ([#177](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/177)) ([b42abd9](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b42abd938b67ca3a3a2ee6cd7f4e6f36e9911702))

## [1.21.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.20.0...octopus-argocd-gateway-chart-v1.21.0) (2026-03-10)


### Features

* argocd grpc-web support ([#168](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/168)) ([09b4acf](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/09b4acfd515bd618088a77e57c2e1efa7c33781e))
* **main:** release 1.17.0 ([#173](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/173)) ([0fcb94a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/0fcb94acc60835c3fee8eca2d143d9c3c4f66340))

## [1.20.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.19.0...octopus-argocd-gateway-chart-v1.20.0) (2026-03-05)


### Features

* add hook weight to registration secret to ensure it's created before pod ([#171](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/171)) ([2f90d2b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2f90d2b2e3900d6a96a99bf137dd39a3f2a1327b))

## [1.19.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.18.0...octopus-argocd-gateway-chart-v1.19.0) (2026-03-05)


### Features

* Consolidate certificate props in helm chart ([#167](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/167)) ([1f85665](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1f85665a7fd0b0085de4f3c017515ef4b5903796))
* **main:** release 1.16.0 ([#169](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/169)) ([513b57f](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/513b57f0b7231bb759c05f4e72b4f0d05612a165))

## [1.18.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.17.0...octopus-argocd-gateway-chart-v1.18.0) (2026-02-27)


### Features

* Add support for external secrets for registration  ([#164](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/164)) ([09b2c55](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/09b2c550fc1da5d1ae53ba317758c06324c7b372))

## [1.17.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.16.0...octopus-argocd-gateway-chart-v1.17.0) (2026-02-16)


### Features

* **main:** release 1.15.0 ([#162](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/162)) ([e3411cf](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/e3411cfc1d0e92e4162ec3ada1cec0472e205d2f))

## [1.16.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.15.0...octopus-argocd-gateway-chart-v1.16.0) (2026-02-02)


### Features

* Allow using external secret for storing Argo CD auth token ([#160](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/160)) ([94885fd](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/94885fd2ad6f74b29930541038401f1327d7b959))
* **main:** release 1.13.0 ([#153](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/153)) ([f3fee44](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f3fee44c64ae208c00493e9ba55d33e5e745a62b))
* **main:** release 1.14.0 ([#159](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/159)) ([c4e5a4f](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/c4e5a4fee27da62f994ccdfc34bcf53c91350334))

## [1.15.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.14.0...octopus-argocd-gateway-chart-v1.15.0) (2025-12-25)


### Features

* **main:** release 1.12.0 ([#151](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/151)) ([216ea4c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/216ea4c6fff44ffcec7c383bd60921d867c7a4ee))

## [1.14.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.13.0...octopus-argocd-gateway-chart-v1.14.0) (2025-12-18)


### Features

* **main:** release 1.11.1 ([#148](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/148)) ([f9654d4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f9654d4b6730d2ee15155b9a2d6da0e66ce6642f))

## [1.13.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.12.0...octopus-argocd-gateway-chart-v1.13.0) (2025-12-11)


### Features

* **main:** release 1.11.0 ([#143](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/143)) ([d06b2dd](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/d06b2dd07a3ce784ccb55fcdeb254cc3fccbe2db))

## [1.12.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.11.0...octopus-argocd-gateway-chart-v1.12.0) (2025-12-08)


### Features

* allow custom certs ([#138](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/138)) ([8fbb9f4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8fbb9f4ea5e0cf80033828eacd4944188abd2ba0))
* **main:** release 1.10.0 ([#140](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/140)) ([1c7bcf4](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1c7bcf4b8a3a75dc0b33eb91b3f382054e3ca8cf))

## [1.11.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.10.0...octopus-argocd-gateway-chart-v1.11.0) (2025-12-02)


### Features

* **main:** release 1.9.1 ([#135](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/135)) ([a3bf0b1](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/a3bf0b1041a7c9b0ef9d9cfb024047553caa0ac7))

## [1.10.0](https://github.com/octopusdeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.9.0...octopus-argocd-gateway-chart-v1.10.0) (2025-11-27)


### Features

* Automatically set thumbprint in configuration ([#129](https://github.com/octopusdeploy/octopus-argocd-gateway/issues/129)) ([da4c63a](https://github.com/octopusdeploy/octopus-argocd-gateway/commit/da4c63a5e94a2c1a320e8114b4806c0c7579fea8))
* **main:** release 1.9.0 ([#131](https://github.com/octopusdeploy/octopus-argocd-gateway/issues/131)) ([7bedca5](https://github.com/octopusdeploy/octopus-argocd-gateway/commit/7bedca5f247349883289f7b880c0270d56d7c945))

## [1.9.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.8.0...octopus-argocd-gateway-chart-v1.9.0) (2025-11-20)


### Features

* **main:** release 1.8.1 ([#127](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/127)) ([e1e5190](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/e1e519084fb608d805d136332e93e58b33c388f6))

## [1.8.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.7.0...octopus-argocd-gateway-chart-v1.8.0) (2025-11-20)


### Features

* **main:** release 1.8.0 ([#124](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/124)) ([293a0fe](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/293a0fe820fb9ac87fc6a8cc194279cb7faee226))

## [1.7.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.6.0...octopus-argocd-gateway-chart-v1.7.0) (2025-11-14)


### Features

* Add new method on InfoService to get the gateway logs ([#118](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/118)) ([545056f](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/545056f9ca288df6bbf3911b4fae4a00ef046052))
* **main:** release 1.7.0 ([#120](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/120)) ([8cc44e3](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8cc44e36887be1b1c5c7e0c0f55b16d51e9b83a1))

## [1.6.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.5.0...octopus-argocd-gateway-chart-v1.6.0) (2025-11-13)


### Features

* **main:** release 1.6.0 ([#117](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/117)) ([77a8a3b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/77a8a3b257ae0dd9651bdfd8b5ce8957417943d7))

## [1.5.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.4.0...octopus-argocd-gateway-chart-v1.5.0) (2025-11-11)


### Features

* **main:** release 1.5.0 ([#113](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/113)) ([2ba0d66](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2ba0d6665899823caf64142d291bf28032cf8c6f))

## [1.4.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.3.0...octopus-argocd-gateway-chart-v1.4.0) (2025-11-04)


### Features

* **main:** release 1.4.0 ([#109](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/109)) ([8eac220](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8eac220bf1fc3e6f70f4d512a4a427d419c06dc0))

## [1.3.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.2.0...octopus-argocd-gateway-chart-v1.3.0) (2025-10-23)


### Features

* **main:** release 1.3.0 ([#106](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/106)) ([2d3d504](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2d3d504b607077f59d8f46e59f7f36e90f0576d0))

## [1.2.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.1.0...octopus-argocd-gateway-chart-v1.2.0) (2025-10-21)


### Features

* **main:** release 1.2.0 ([#103](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/103)) ([257c678](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/257c6781b48d58b9ef94ca080395811f0fe78fe2))

## [1.1.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v1.0.0...octopus-argocd-gateway-chart-v1.1.0) (2025-10-14)


### Features

* Add plaintext configuration option to disable TLS ([#99](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/99)) ([6e8f5e2](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/6e8f5e2ab7f77ac795323414d3b7adfb21f1fa93))
* **main:** release 1.1.0 ([#100](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/100)) ([1ab59ed](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1ab59edd8bce081c6b693b5bba6eb248c28ce913))

## [1.0.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.21.0...octopus-argocd-gateway-chart-v1.0.0) (2025-10-02)


### Features

* **main:** release 1.0.0 ([#94](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/94)) ([6469990](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/6469990dd7f756248289435e3cf7c2bae912c4e8))

## [0.21.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.20.0...octopus-argocd-gateway-chart-v0.21.0) (2025-09-24)


### Features

* **main:** release 0.12.0 ([#91](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/91)) ([8ac1ffc](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/8ac1ffc3ed66c6d33fc2b9eeb35329fe9ff97ded))
* Pass through web ui url through to registration api call ([#90](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/90)) ([5f373ee](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/5f373eea648453734cb41c11c8cb3e09100d73a6))

## [0.20.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.19.0...octopus-argocd-gateway-chart-v0.20.0) (2025-09-14)


### Features

* **main:** release 0.11.0 ([#88](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/88)) ([2dc8149](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2dc81490603e7a68130286718791e29d556df464))

## [0.19.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.18.0...octopus-argocd-gateway-chart-v0.19.0) (2025-08-29)


### Features

* **main:** release 0.10.0 ([#85](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/85)) ([59d7b14](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/59d7b14a43301496c90b576ee4edb91ffd925937))

## [0.18.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.17.0...octopus-argocd-gateway-chart-v0.18.0) (2025-08-20)


### Features

* **main:** release 0.9.0 ([#82](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/82)) ([4bc0584](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/4bc0584616f9dc901124aeac5065c7e2417e3e1d))

## [0.17.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.16.0...octopus-argocd-gateway-chart-v0.17.0) (2025-08-08)


### Features

* Add adjustable JobsHistoryLimits to auto-update job ([#80](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/80)) ([264cbaa](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/264cbaa50e852aa4d56be21aa6d46bcc8439007a))

## [0.16.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.15.0...octopus-argocd-gateway-chart-v0.16.0) (2025-08-05)


### Features

* **main:** release 0.8.0 ([#75](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/75)) ([fb58479](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/fb5847999b71864098d1479327809932bafd8b40))

## [0.15.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.14.0...octopus-argocd-gateway-chart-v0.15.0) (2025-08-04)


### Features

* Add CronJob to auto update the Helm chart ([#70](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/70)) ([368640d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/368640d02a7e6f4d22505da86bb8e8d6097aeed9))

## [0.14.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.13.0...octopus-argocd-gateway-chart-v0.14.0) (2025-07-31)


### Features

* Add new info service ([#67](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/67)) ([4cdc48c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/4cdc48cca2ec1c1433143f619623484dedf04405))
* **main:** release 0.7.0 ([#71](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/71)) ([7c92767](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/7c92767cfab7d40a38f56abd952094c416b9b9da))

## [0.13.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.12.0...octopus-argocd-gateway-chart-v0.13.0) (2025-07-18)


### Features

* Improve startup error handling and post install upgrades ([#60](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/60)) ([1d6e39e](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/1d6e39e5589ab942d000fdba1026a24c79dfc0e0))
* **main:** release 0.6.0 ([#65](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/65)) ([87d8976](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/87d8976874241ee5c9925b4a5a9b72ce554d3567))

## [0.12.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.11.0...octopus-argocd-gateway-chart-v0.12.0) (2025-07-11)


### Features

* Add support for non-secure connections to ArgoCD ([#61](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/61)) ([f2f952c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f2f952c8ec9730094866c4750805d866973ca476))
* **main:** release 0.5.0 ([#62](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/62)) ([f09c434](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/f09c434816dfacedfa30dac595057be046c34fe4))

## [0.11.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.10.0...octopus-argocd-gateway-chart-v0.11.0) (2025-07-02)


### Features

* **main:** release 0.4.2 ([#58](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/58)) ([632171d](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/632171d5bde59c43975c348355386ab830e61a02))

## [0.10.0](https://github.com/octopusdeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.9.0...octopus-argocd-gateway-chart-v0.10.0) (2025-06-20)


### Features

* **main:** release 0.4.0 ([#52](https://github.com/octopusdeploy/octopus-argocd-gateway/issues/52)) ([dd02344](https://github.com/octopusdeploy/octopus-argocd-gateway/commit/dd02344861f4dd3c26b754934c1e0cd0484fc52a))
* **main:** release 0.4.1 ([#54](https://github.com/octopusdeploy/octopus-argocd-gateway/issues/54)) ([274e334](https://github.com/octopusdeploy/octopus-argocd-gateway/commit/274e334816014d4bc38cc520321df573bc98defe))


### Bug Fixes

* Removed release as and updated chart.yaml to include more details ([#53](https://github.com/octopusdeploy/octopus-argocd-gateway/issues/53)) ([74a6fd4](https://github.com/octopusdeploy/octopus-argocd-gateway/commit/74a6fd46c04f49a2824d45767e9ad03ebfab9054))


### Miscellaneous Chores

* release 0.4.0 ([#45](https://github.com/octopusdeploy/octopus-argocd-gateway/issues/45)) ([074a3e2](https://github.com/octopusdeploy/octopus-argocd-gateway/commit/074a3e25769e4d4c681f55a91eae1371dd12440a))

## [0.9.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.9.0...octopus-argocd-gateway-chart-v0.9.0) (2025-06-20)


### Features

* **main:** release 0.4.0 ([#52](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/52)) ([dd02344](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/dd02344861f4dd3c26b754934c1e0cd0484fc52a))


### Miscellaneous Chores

* release 0.4.0 ([#45](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/45)) ([074a3e2](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/074a3e25769e4d4c681f55a91eae1371dd12440a))

## [0.9.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-chart-v0.8.0...octopus-argocd-gateway-chart-v0.9.0) (2025-06-17)


### Features

* Tidied up helm chart ([#42](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/42)) ([2196ded](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2196ded868b6bc5f6f77a304fc8fca1c1c3d2a10))

## [0.8.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.7.0...octopus-argocd-gateway-v0.8.0) (2025-06-17)


### Features

* **main:** release 0.3.2 ([#36](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/36)) ([4b12460](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/4b12460c944caa60839e9ef7c3d1eec568549f64))

## [0.7.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.6.0...octopus-argocd-gateway-v0.7.0) (2025-06-13)


### Features

* **main:** release 0.3.1 ([#32](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/32)) ([145ec51](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/145ec51a132b1ec600754c6cffd8b9fac096935d))


### Bug Fixes

* Image tags require `v` prefix . Also sanitize gRPC urls ([#31](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/31)) ([15af64b](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/15af64bb7438c9b9fe141a6470bf13590f0a814c))

## [0.6.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.5.0...octopus-argocd-gateway-v0.6.0) (2025-06-12)


### Features

* **main:** release 0.3.0 ([#29](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/29)) ([e0643cb](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/e0643cbb6199687b8ee2a91cc3a0ec8d1b4bd386))

## [0.5.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.4.0...octopus-argocd-gateway-v0.5.0) (2025-06-12)


### Features

* Support registration via helm chart ([#25](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/25)) ([87b68d9](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/87b68d9f1d49774abf51ab49be5f21aa5fb068e5))


### Bug Fixes

* Make chart and app version static in snapshot tests ([#28](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/28)) ([2ec38ce](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/2ec38ce4bd21e0253d022c3dffec66bb9ebac8c6))

## [0.4.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.3.0...octopus-argocd-gateway-v0.4.0) (2025-06-04)


### Features

* **main:** release 0.2.0 ([#20](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/20)) ([96df70c](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/96df70cc5a609145c351ff4d8c815f0784bdc585))

## [0.3.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.2.0...octopus-argocd-gateway-v0.3.0) (2025-06-04)


### Features

* **main:** release 0.1.0 ([#15](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/15)) ([691858a](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/691858a90ce98a1e8a8e80a4723d647bc77a9976))

## [0.2.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.1.0...octopus-argocd-gateway-v0.2.0) (2025-05-16)


### Features

* connect to ArgoCD ([#13](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/13)) ([9149a22](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/9149a22d0bd78597840d7a17d1f856406fd74ef5))

## [0.1.0](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.0.1...octopus-argocd-gateway-v0.1.0) (2025-05-13)


### Features

* **main:** release 0.0.1 ([#11](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/11)) ([71f1bd2](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/71f1bd20fc4ea2709618329623ac6068e4b6eab2))

## [0.0.1](https://github.com/OctopusDeploy/octopus-argocd-gateway/compare/octopus-argocd-gateway-v0.0.1...octopus-argocd-gateway-v0.0.1) (2025-05-13)


### Miscellaneous Chores

* release 0.0.1 ([#3](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/3)) ([1412361](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/14123611eb36eddcb335a0d655cc82ed833d52f4))
* release 0.0.2 ([#6](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/6)) ([56151ce](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/56151ce9be1a2a8570af3590191459e68136b32d))
* rename to argocd ([#9](https://github.com/OctopusDeploy/octopus-argocd-gateway/issues/9)) ([b5718a0](https://github.com/OctopusDeploy/octopus-argocd-gateway/commit/b5718a0962725da6f88c3bd6e77cbccb631258c7))
