# Troubleshooting Verification Failures

This guide helps diagnose and resolve common issues encountered when running `tsuku verify` on installed tools and libraries.

## Understanding Verification Tiers

For libraries, `tsuku verify` performs four tiers of validation:

- **Tier 1: Header validation** - Validates library files are valid shared libraries (ELF/Mach-O)
- **Tier 2: Dependency checking** - Validates dynamic library dependencies are satisfied
- **Tier 3: dlopen load testing** - Verifies the library can be dynamically loaded at runtime
- **Tier 4: Integrity verification** - Compares checksums against installation-time values

This guide focuses on troubleshooting failures in each tier.

## Tier 2: Dependency Validation Failures

### Symptom: Missing tsuku-managed dependency

```
Error: Dependency validation failed: libfoo.so.1 not found (TSUKU_MANAGED)
```

**Cause**: A library dependency that tsuku should provide is not installed.

**Solution**:
```bash
# Install the missing library
tsuku install libfoo

# Retry verification
tsuku verify yourtool
```

### Symptom: ABI mismatch (glibc vs musl)

```
Error: ABI validation failed: interpreter /lib64/ld-linux-x86-64.so.2 not found
```

**Cause**: The binary was compiled for glibc but you're running on a musl-based system (e.g., Alpine Linux), or vice versa.

**Solutions**:

1. **Use a compatible binary:**
   ```bash
   # On Alpine/musl systems, look for musl-specific recipes
   tsuku search python-musl
   tsuku install python-musl
   ```

2. **Install on a compatible system:**
   - glibc binaries require glibc-based distributions (Ubuntu, Debian, Fedora, etc.)
   - musl binaries require musl-based distributions (Alpine Linux)

3. **Report the issue:**
   If no compatible recipe exists, open an issue requesting one.

### Symptom: Unknown dependency

```
Error: Dependency validation failed: libunknown.so.1 - UNKNOWN
```

**Cause**: The library depends on a shared library that:
- Isn't a recognized system library
- Isn't provided by tsuku
- Isn't from an external package manager

**Solutions**:

1. **Check if it's a new system library pattern:**
   The system library registry may need updating. Report this to tsuku maintainers with your platform details.

2. **Install manually if needed:**
   Some tools have optional dependencies that aren't automatically detected. Check the tool's documentation.

3. **Verify the binary:**
   The binary may be malformed or built incorrectly. Try reinstalling:
   ```bash
   tsuku remove yourtool
   tsuku install yourtool
   ```

### Symptom: Limited validation for old library installations

```
Warning: Library installed before Tier 2 support - limited validation available
```

**Cause**: Libraries installed before M38 don't have stored soname metadata.

**Solution**: Reinstall the library to populate metadata:
```bash
tsuku remove libfoo
tsuku install libfoo
```

## Tier 1: Header Validation Failures

### Symptom: Invalid ELF/Mach-O header

```
Error: Header validation failed: invalid ELF magic number
```

**Cause**: The file is corrupted or not a valid shared library.

**Solution**: Reinstall the tool/library:
```bash
tsuku remove yourtool
tsuku install yourtool
```

### Symptom: Architecture mismatch

```
Error: Header validation failed: wrong architecture (expected arm64, got x86_64)
```

**Cause**: The binary doesn't match your system architecture.

**Solution**: This usually indicates a recipe bug. Report the issue with your platform details (OS, architecture).

## Tier 3: dlopen Load Testing Failures

### Symptom: Symbol not found

```
Error: dlopen failed: symbol 'foo_function' not found
```

**Cause**: A required symbol is missing from a dependency library.

**Solution**:
1. Ensure all dependencies are up to date:
   ```bash
   tsuku update yourtool
   ```

2. If the issue persists, this may indicate ABI incompatibility. Report the issue with version details.

### Symptom: Permission denied

```
Error: dlopen failed: permission denied
```

**Cause**: The library file lacks execute permissions or is in a non-executable filesystem.

**Solution**:
```bash
# Fix permissions
chmod +x $TSUKU_HOME/tools/yourtool-1.0.0/lib/libfoo.so

# Or reinstall
tsuku remove yourtool
tsuku install yourtool
```

## Tier 4: Integrity Verification Failures

### Symptom: Checksum mismatch

```
Error: Integrity check failed: libfoo.so.1 checksum mismatch
Expected: abc123...
Got: def456...
```

**Cause**: The library file has been modified since installation (tampering, corruption, or upgrade).

**Solutions**:

1. **Reinstall if unexpected:**
   ```bash
   tsuku remove yourtool
   tsuku install yourtool
   ```

2. **If you intentionally modified the file:**
   This is expected behavior. Tier 4 detects post-installation changes. You can skip integrity checks:
   ```bash
   tsuku verify yourtool --skip-integrity
   ```

## General Troubleshooting

### Enable verbose output

```bash
tsuku verify yourtool --verbose
```

This shows detailed validation steps and dependency tree traversal.

### Check installation state

```bash
# View installed tools and libraries
tsuku list

# Check specific tool/library details
tsuku info yourtool
```

### Verify tsuku itself

```bash
# Ensure tsuku is working correctly
tsuku doctor
```

## Getting Help

If you encounter issues not covered here:

1. **Check design documents** for technical details:
   - `docs/designs/current/DESIGN-library-verify-deps.md` - Tier 2 architecture
   - `docs/designs/current/DESIGN-library-verify-dlopen.md` - Tier 3 architecture
   - `docs/designs/current/DESIGN-library-verify-integrity.md` - Tier 4 architecture

2. **Search existing issues**: https://github.com/tsukumogami/tsuku/issues

3. **Open a new issue** with:
   - Output of `tsuku verify yourtool --verbose`
   - Your platform (`uname -a`)
   - Tsuku version (`tsuku --version`)
   - Steps to reproduce
