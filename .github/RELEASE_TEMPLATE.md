# Injective $${{TAG}} Mainnet Upgrade 🥷

📋 [Full Changelog](https://github.com/InjectiveFoundation/injective-core/releases/download/$${{RELEASE_TAG}}/CHANGELOG.md)

<!--
## Source Code

* Tag: [$${{TAG}}](https://github.com/InjectiveFoundation/injective-core/tree/$${{TAG}})
* Branch: [release/v1.19.x](https://github.com/InjectiveFoundation/injective-core/tree/release/v1.19.x)
-->

## Upgrade Guide

Validators can find a step-by-step guide on the upgrade procedure in the long-form documentation: <https://docs.injective.network/infra/validator-mainnet/canonical-chain-upgrade-$${{TAG}}>

### Versions

| Binary    | Version | Code Commit|
| -------- | ------- |------- |
| injectived  | $${{TAG}}   |`$${{SOURCE_COMMIT}}`|
| peggo  | $${{TAG}} |`$${{SOURCE_COMMIT}}`|

`Go version 1.26.4`

Verify you're using the correct version by running the below commands:

```bash
injectived version
peggo version
```

```bash
docker run -it --rm injectivelabs/injective-core:$${{TAG}} injectived version
docker run -it --rm injectivelabs/injective-core:$${{TAG}} peggo version
```

Results:

```
[A] injectived
Version $${{TAG}} ($${{SOURCE_COMMIT}})
Compiled at $${{INJECTIVED_COMPILED_AT}} using Go go1.26.4 (amd64)

[B] peggo
Version $${{TAG}} ($${{SOURCE_COMMIT}})
Compiled at $${{PEGGO_COMPILED_AT}} using Go go1.26.4 (amd64)
```

### 🐳 Docker

Docker image have support for both `amd64` and `arm64` architectures.

| Image    | Description |
| -------- | ------- |
| injectivelabs/injective-core:$${{TAG}} | Debian image |

### 🕊️ Download Binaries

```bash
wget https://github.com/InjectiveFoundation/injective-core/releases/download/$${{RELEASE_TAG}}/linux-amd64.zip
unzip linux-amd64.zip
sudo mv injectived peggo /usr/bin
sudo mv libwasmvm.x86_64.so /usr/lib
```
