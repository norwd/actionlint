### Error at line 3, col 5 of `testdata/format/test.yaml`

unexpected key "branch" for "push" section. expected one of "branches", "branches-ignore", "paths", "paths-ignore", "tags", "tags-ignore", "types", "workflows"

```
    branch: main
    ^~~~~~~
```

### Error at line 9, col 23 of `testdata/format/test.yaml`

property "msg" is not defined in object type {}

```
      - run: echo ${{ matrix.msg }}
                      ^~~~~~~~~~
```

### Error at line 10, col 9 of `testdata/format/test.yaml`

this step is for running shell command since it contains at least one of "run", "shell" keys, but also contains "with" key which is used for running action

```
        with:
        ^~~~~
```

