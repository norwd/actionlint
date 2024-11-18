<a id="hello"></a>
## Hello

Example input:

```yaml
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ unknown }}
```

Output:

```
test.yaml:6:23: undefined variable "unknown". available variables are "env", "github", "inputs", "job", "matrix", "needs", "runner", "secrets", "steps", "strategy", "vars" [expression]
  |
6 |       - run: echo ${{ unknown }}
  |                       ^~~~~~~
```

<a id="hello2"></a>
## Hello2

...
