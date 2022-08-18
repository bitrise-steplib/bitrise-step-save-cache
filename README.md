# Save Cache (Beta)

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/bitrise-step-save-cache?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/bitrise-step-save-cache/releases)

Saves build cache using a cache key. This Step needs to be used in combination with **Restore Cache**.

<details>
<summary>Description</summary>

Saves build cache using a cache key. This Step needs to be used in combination with **Restore Cache**.

**Beta status**: while this step is in beta, there are no usage restrictions or costs associated with using cache in builds.

#### About key-based caching

Key-based caching is a concept where cache archives are saved and restored using a unique cache key. One Bitrise project can have multiple cache archives stored simultaneously, and the **Restore Cache Step** downloads a cache archive associated with the key provided as a Step input. The **Save Cache** step is responsible for uploading the cache archive with an exact key.

Caches can become outdated across builds when something changes in the project (for example, a dependency gets upgraded to a new version). In this case, a new (unique) cache key is needed to save the new cache contents. This is possible if the cache key is dynamic and changes based on the project state (for example, a checksum of the dependency lockfile is part of the cache key). If you use the same dynamic cache key when restoring the cache, the Step will download the most relevant cache archive available.

Key-based caching is platform-agnostic and can be used to cache anything by carefully selecting the cache key and the files/folders to include in the cache.

#### Templates

The Step requires a string key to use when uploading a cache archive. In order to always download the most relevant cache archive for each build, the cache key input can contain template elements. The **Restore cache Step** evaluates the key template at runtime and the final key value can change based on the build environment or files in the repo. Similarly, the **Save cache** step also uses templates to compute a unique cache key when uploading a cache archive.

The following variables are supported in the cache key input:

- `cache-key-{{ .Branch }}`: Current git branch the build runs on
- `cache-key-{{ .CommitHash }}`: SHA-256 hash of the git commit the build runs on
- `cache-key-{{ .Workflow }}`: Current Bitrise workflow name (eg. `primary`)
- `{{ .Arch }}-cache-key`: Current CPU architecture (`amd64` or `arm64`)
- `{{ .OS }}-cache-key`: Current operating system (`linux` or `darwin`)

Functions available in a template:

`checksum`: This function takes one or more file paths and computes the SHA256 [checksum](https://en.wikipedia.org/wiki/Checksum) of the file contents. This is useful for creating unique cache keys based on files that describe content to cache.

Examples of using `checksum`:
- `cache-key-{{ checksum "package-lock.json" }}`
- `cache-key-{{ checksum "**/Package.resolved" }}`
- `cache-key-{{ checksum "**/*.gradle*" "gradle.properties" }}`

`getenv`: This function returns the value of an environment variable or an empty string if the variable is not defined.

Examples of `getenv`:
- `cache-key-{{ getenv "PR" }}`
- `cache-key-{{ getenv "BITRISEIO_PIPELINE_ID" }}`

#### Key matching

The most straightforward use case is when both the **Save cache** and **Restore cache** Steps use the same exact key to transfer cache between builds. Stored cache archives are scoped to the Bitrise project. Builds can restore caches saved by any previous Workflow run on any Bitrise Stack.

Unlike this Step, the **Restore cache** Step can define multiple keys as fallbacks when there is no match for the first cache key. See the docs of the **Restore cache** Step for more details.

#### Related steps

[Restore cache](https://github.com/bitrise-steplib/bitrise-step-restore-cache/)

</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

### Examples

#### Skip saving the cache in PR builds (only restore)

```yaml
steps:
- restore-cache@1:
    inputs:
    - key: node-modules-{{ checksum "package-lock.json" }}

# Build steps

- save-cache@1:
    run_if: ".IsCI | and (not .IsPR)" # Condition that is false in PR builds
    inputs:
    - key: node-modules-{{ checksum "package-lock.json" }}
    - paths: node_modules
```

#### Separate caches for each OS and architecture

Cache is not guaranteed to work across different Bitrise Stacks (different OS or same OS but different CPU architecture). If a workflow runs on different stacks, it's a good idea to include the OS and architecture in the cache key:

```yaml
steps:
- save-cache@1:
    inputs:
    - key: '{{ .OS }}-{{ .Arch }}-npm-cache-{{ checksum "package-lock.json" }}'
    - path: node_modules
```

#### Multiple independent caches

You can add multiple instances of this step to a workflow:

```yaml
steps:
- save-cache@1:
    title: Save NPM cache
    inputs:
    - paths: node_modules
    - key: node-modules-{{ checksum "package-lock.json" }}
- save-cache@1:
    title: Save Python cache
    inputs:
    - paths: venv/
    - key: pip-packages-{{ checksum "requirements.txt" }}
```


## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `key` | Key used for saving a cache archive.  The key supports template elements for creating dynamic cache keys. These dynamic keys change the final key value based on the build environment or files in the repo in order to create new cache archives.  See the Step description for more details and examples. | required |  |
| `paths` | List of files and folders to include in the cache.  The path can contain wildcards (`*` and `**`) that are evaluated at runtime. | required |  |
| `verbose` | Enable logging additional information for troubleshooting | required | `false` |
</details>

<details>
<summary>Outputs</summary>
There are no outputs defined in this step
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/bitrise-step-save-cache/pulls) and [issues](https://github.com/bitrise-steplib/bitrise-step-save-cache/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

**Note:** this step's end-to-end tests (defined in `e2e/bitrise.yml`) are working with secrets which are intentionally not stored in this repo. External contributors won't be able to run those tests. Don't worry, if you open a PR with your contribution, we will help with running tests and make sure that they pass.


Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
