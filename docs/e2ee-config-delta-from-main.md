# E2EE & config vs main branch

## 1. GOJOPLIN_MASTER_PASSWORD parsing and injection

### Main branch
- **Source:** Only from CLI override (`--master-password`) or env `GOJOPLIN_MASTER_PASSWORD`. No config file key for master password (Joplin `settings.json` has no such key).
- **Value:** Used **as-is** (no trimming). Whatever is in the env or flag is passed to the sync engine and then to E2EE when loading master keys.

### This branch (mcp/allow-actions)
- **Sources:**
  - **YAML:** `api.master_password` with `${VAR}` expansion (e.g. `master_password: "${GOJOPLIN_MASTER_PASSWORD}"`). Set in `loadFromYAML()` and trimmed with `TrimSpace`.
  - **JSON (Joplin):** Still no master password in the file; only overrides and env.
  - **Overrides and env:** Applied in `applyOverridesAndEnv()` in `internal/config/yaml.go`. Previously both override and env values were **TrimSpace’d** for master password (as well as username, password, API key).

- **Behavioral difference:** On this branch, `GOJOPLIN_MASTER_PASSWORD` (and override) were passed through `strings.TrimSpace()`. On main, the value is used raw. If the desktop client uses a master password that has leading/trailing spaces (or the desktop stores it without trim), and the server trims it, the server would derive a different master key and encrypt notes with that key. The desktop would then fail to decrypt those notes.

**Fix applied:** Master password is no longer trimmed when applied from overrides or env, so behavior matches main. YAML `master_password` is still trimmed when read from the config file (to avoid newlines from `${VAR}` expansion); if you rely on leading/trailing space in the password, set it via env or CLI, not via YAML.

---

## 2. Encryption of notes

- **No code changes** between main and this branch in:
  - `internal/e2ee/` (encrypt/decrypt, master key loading, StringV1/FileV1, JED01)
  - `internal/sync/push.go` (serialize note/folder/tag, encrypt before push, active master key ID)
  - `internal/sync/engine.go` (use of master password for decryption only)

- **Flow (same on both branches):**
  - Master password is used only to **decrypt** master keys (from sync info / DB). It is not used to encrypt note content.
  - Note encryption uses the **decrypted master key** (cached in `e2ee.Service`) and the same **active master key ID** from sync info. Cipher format is JED01, method StringV1 (method 10), same chunking and PBKDF2/AES-GCM as before.

So **encryption format and key usage are identical**. Any desktop decryption errors for notes created on this branch are therefore not due to a different encryption algorithm or format, but to a **different effective master password** (e.g. trim vs no-trim) leading to a different decrypted master key on the server than on the desktop.

---

## Summary

| Aspect | Main | This branch (after fix) |
|--------|------|--------------------------|
| Master password from env | Raw (no trim) | Raw (no trim) — matches main |
| Master password from YAML | N/A | TrimSpace when read from file only |
| Note encryption code | Unchanged | Unchanged |
| Note encryption format | JED01 StringV1 | JED01 StringV1 (same) |

Ensure the master password used by the server (env or CLI) is **exactly** the same as on the desktop (including any spaces). After the fix, we no longer trim the env/CLI value so behavior matches main.
