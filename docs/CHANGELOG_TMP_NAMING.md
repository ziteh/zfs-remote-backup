# Temporary File Naming Improvement

**Date**: 2026-02-07
**Type**: Bug Fix + Enhancement
**Impact**: Low (internal implementation detail)

---

## Problem

The original temporary file naming was confusing and had a rename logic bug:

```bash
# Original implementation
split creates:  snapshot.part-.tmpaaaaaa
                snapshot.part-.tmpaaaaab
                snapshot.part-.tmpaaaaac

Rename logic:   strings.TrimSuffix(tmpFile, ".tmp")
Result:         Files NOT renamed (TrimSuffix failed because suffix is ".tmpaaaaaa" not ".tmp")
```

The files were named `snapshot.part-.tmpaaaaaa` which mixed the temporary marker (`.tmp`) with the split suffix, making it unclear what was temporary.

---

## Solution

Changed to a clearer pattern using `--additional-suffix` for split:

```bash
# New implementation
split creates:  snapshot.part-aaaaaa.tmp
                snapshot.part-aaaaab.tmp
                snapshot.part-aaaaac.tmp

Rename logic:   strings.TrimSuffix(tmpFile, ".tmp")
Result:         snapshot.part-aaaaaa
                snapshot.part-aaaaab
                snapshot.part-aaaaac
```

After encryption:
```bash
                snapshot.part-aaaaaa.age
                snapshot.part-aaaaab.age
                snapshot.part-aaaaac.age
```

---

## Changes Made

### File: `simple_backup/zfs.go`

1. **Line 26**: Remove `.tmp` prefix from outputPatternTmp
   ```go
   // Before
   outputPatternTmp := filepath.Join(exportDir, "snapshot.part-.tmp")

   // After
   outputPatternTmp := filepath.Join(exportDir, "snapshot.part-")
   ```

2. **Line 54**: Add `--additional-suffix=.tmp` to split command
   ```go
   // Before
   splitCmd := exec.CommandContext(ctx, "split", "-b", "3G", "-a", "6", "-", outputPatternTmp)

   // After
   splitCmd := exec.CommandContext(ctx, "split", "-b", "3G", "-a", "6", "--additional-suffix=.tmp", "-", outputPatternTmp)
   ```

3. **Line 32**: Update cleanup glob pattern
   ```go
   // Before
   matches, _ := filepath.Glob(outputPatternTmp + "*")

   // After
   matches, _ := filepath.Glob(outputPatternTmp + "*.tmp")
   ```

4. **Line 143**: Update rename glob pattern
   ```go
   // Before
   matches, err := filepath.Glob(outputPatternTmp + "*")

   // After
   matches, err := filepath.Glob(outputPatternTmp + "*.tmp")
   ```

---

## Testing

Created comprehensive test: `vm/tests/09_test_tmp_naming.sh`

**Test Results**:
```
✓ Test 5: Backup creates .tmp files correctly
✓ Test 6: Final files have no .tmp suffix
✓ Test 7: No .tmp files remain after rename
✓ Test 8: Encrypted files have correct format (.age)
```

**Log Evidence**:
```
level=DEBUG msg="Renamed tmp file"
  tmpFile=.../snapshot.part-aaaaaa.tmp
  finalFile=.../snapshot.part-aaaaaa
```

---

## Benefits

1. **Clarity**: Clearly indicates temporary files with `.tmp` suffix
2. **Correctness**: Rename logic now works properly
3. **Consistency**: Standard naming pattern (base + suffix)
4. **Debugging**: Easier to identify file states during backup

---

## Backward Compatibility

**No impact on existing backups**:
- Only affects temporary file naming during backup process
- Final encrypted files (`.age`) have same naming pattern
- Manifests and restore logic unchanged

---

## Production Status

✅ **Ready for Production**
- Change tested and verified
- No breaking changes
- Improves code clarity and correctness
