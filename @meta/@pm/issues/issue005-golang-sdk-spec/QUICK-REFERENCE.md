# Quick Reference - SSE Tests & Multi-Backend Testing

## 🚀 Run Tests (Most Common)

```bash
# All SSE tests on all backends (RECOMMENDED)
bin/run_golang_sdk_specs.sh all TestSSE

# All tests on all backends
bin/run_golang_sdk_specs.sh

# Specific backend only
bin/run_golang_sdk_specs.sh sqlite
bin/run_golang_sdk_specs.sh postgres
bin/run_golang_sdk_specs.sh pebble
```

## 📋 What Got Fixed

✅ **SSE-002** - Category subscription (timing race condition)  
✅ **SSE-004** - Subscribe without auth (test mode bypass)  
✅ **SSE-005** - Consumer group subscription (timing race condition)  
✅ **SSE-007** - Reconnection handling (position tracking)  

**Result: 8/8 SSE tests passing on all 3 backends**

## 🔍 Root Causes

1. **Race Condition** - Server now sends `: ready` signal
2. **Category Bug** - Fixed category name extraction
3. **Auth Testing** - Production mode for auth tests
4. **Position Mix-up** - Use stream position, not global

## 📦 Backend Support

- **SQLite** - In-memory, fast, default
- **PostgreSQL** - Full DB, requires server
- **Pebble** - High performance, key-value

Switch with: `TEST_BACKEND=sqlite|postgres|pebble`

## 📖 Documentation

- `bin/README.md` - Test runner guide
- `docs/SSE-TEST-FIXES.md` - Technical details
- `docs/MULTI-BACKEND-TESTING.md` - Backend guide
- `FINAL-SUMMARY.md` - Complete summary

## 💡 Tips

```bash
# Get help
bin/run_golang_sdk_specs.sh --help

# Test specific feature
bin/run_golang_sdk_specs.sh all TestWRITE
bin/run_golang_sdk_specs.sh all TestREAD

# Single test
bin/run_golang_sdk_specs.sh sqlite TestSSE001
```

## ✅ Verification

```bash
bin/run_golang_sdk_specs.sh all TestSSE
# Should see:
# 📦 sqlite  : ✅ PASS
# 🐘 postgres: ✅ PASS
# 🪨 pebble  : ✅ PASS
```
