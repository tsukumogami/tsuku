## Problem

fontconfig binaries fail at runtime on macOS with:

```
dyld: Library not loaded: @rpath/libintl.8.dylib
```

fontconfig depends on gettext (which provides `libintl`). gettext installs correctly to `$TSUKU_HOME/libs/gettext-{version}/`, but fontconfig's binaries reference `@rpath/libintl.8.dylib` and the rpath doesn't include gettext's library directory.

The issue is in the binary wrapper generation or the `homebrew_relocate` step. When a homebrew-installed tool has runtime library dependencies, the wrapper doesn't set `DYLD_LIBRARY_PATH` or fix the rpath to point at the dependency's lib directory under `$TSUKU_HOME/libs/`.

## Reproduction

1. On macOS, run: `tsuku install fontconfig`
2. Both fontconfig and gettext (runtime dependency) install successfully
3. Run any fontconfig binary, e.g.: `fc-list`
4. Fails with: `dyld: Library not loaded: @rpath/libintl.8.dylib`

## Expected behavior

The binary wrapper should include the library paths of runtime dependencies so dynamically linked libraries are found at runtime. Either:

- The wrapper sets `DYLD_LIBRARY_PATH` to include `$TSUKU_HOME/libs/<dep>-<version>/lib` for each runtime dependency, or
- The `homebrew_relocate` step rewrites the rpath in the binary to include those paths via `install_name_tool -add_rpath`

## Affected area

This likely affects any homebrew-based recipe with runtime library dependencies that use dynamic linking on macOS. fontconfig is the first case encountered, but any recipe that depends on a library (not just a binary) from another recipe would hit the same issue.

## Notes

- gettext installs its libraries correctly; the problem is that fontconfig doesn't know where to find them
- On Linux this isn't an issue because `LD_LIBRARY_PATH` or `RPATH` handling works differently with the ELF format
