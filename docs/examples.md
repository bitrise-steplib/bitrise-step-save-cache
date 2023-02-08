### Examples

Check out [Workflow Recipes](https://github.com/bitrise-io/workflow-recipes#-key-based-caching-beta) for platform-specific examples!

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

Cache is not guaranteed to work across different Bitrise Stacks (different OS or same OS but different CPU architecture). If a Workflow runs on different stacks, it's a good idea to include the OS and architecture in the **Cache key** input:

```yaml
steps:
- save-cache@1:
    inputs:
    - key: '{{ .OS }}-{{ .Arch }}-npm-cache-{{ checksum "package-lock.json" }}'
    - path: node_modules
```

#### Multiple independent caches

You can add multiple instances of this Step to a Workflow:

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
