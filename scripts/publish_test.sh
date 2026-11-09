#!/usr/bin/env bash
# Проверяет публикацию через подставной CLI: аргументы, выходы и сводку.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
publish="${script_dir}/../scripts/publish.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

failures=0
check() {
	if ! "$@"; then
		printf 'FAIL: %s\n' "$1" >&2
		failures=$((failures + 1))
	fi
}

cat >"${work}/rustore-publisher" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"${FAKE_ARGS_FILE:-/dev/null}"
if [ -n "${FAKE_CREDENTIALS_EXPECT:-}" ]; then
	previous=""
	for arg in "$@"; do
		if [ "$previous" = "--credentials" ]; then
			[ -s "$arg" ] || exit 9
			grep -q "$FAKE_CREDENTIALS_EXPECT" "$arg" || exit 9
		fi
		previous="$arg"
	done
fi
cat "${FAKE_RESULT_FILE}"
exit "${FAKE_EXIT_CODE:-0}"
EOF
chmod 0755 "${work}/rustore-publisher"
printf 'aab' >"${work}/app.aab"
printf '{"keyId":"key-1","privateKey":"secret"}' >"${work}/creds.json"

result_file="${work}/result.json"
args_file="${work}/args.txt"
cat >"$result_file" <<'EOF'
{
  "schema_version": 1,
  "command": "publish",
  "package": "com.example.app",
  "version_code": 42,
  "version_id": 100,
  "phase": "status",
  "outcome": "submitted",
  "version_status": "TAKEN_FOR_MODERATION",
  "testing_type": "release",
  "submitted": true,
  "published": false,
  "skipped": false,
  "manual_action": false,
  "outcome_unknown": false,
  "warnings": [{"code": "CHANGELOG_INHERITED", "message": "Нет списка изменений."}],
  "error": null
}
EOF

run_publish() {
	FAKE_RESULT_FILE="$result_file" \
		FAKE_ARGS_FILE="$args_file" \
		RUSTORE_TEST_ONLY_BINARY="${work}/rustore-publisher" \
		RUSTORE_VERSION=0.0.1 \
		RUSTORE_AAB="${work}/app.aab" \
		RUSTORE_PACKAGE=com.example.app \
		RUSTORE_API_URL=https://api.example.test \
		RUSTORE_NO_EMBEDDED_CA=true \
		RUSTORE_CREDENTIALS_FILE="${work}/creds.json" \
		RUSTORE_METADATA_DIR=metadata \
		RUSTORE_VERSION_CODE=42 \
		RUSTORE_TIMEOUT=30m \
		RUSTORE_REQUEST_TIMEOUT=2m \
		RUSTORE_PROCESSING_TIMEOUT=2m \
		RUSTORE_DRY_RUN=true \
		RUSTORE_YES=true \
		RUSTORE_DEBUG=false \
		GITHUB_WORKSPACE="$work" \
		GITHUB_OUTPUT="${work}/output" \
		GITHUB_STEP_SUMMARY="${work}/summary" \
		bash "$publish"
}

: >"${work}/output"
: >"${work}/summary"
if ! run_publish; then
	printf 'FAIL: успешная публикация завершилась ошибкой\n' >&2
	exit 1
fi

grep -q '^version-code=42$' "${work}/output" || { printf 'FAIL: version-code\n' >&2; failures=$((failures + 1)); }
grep -q '^version-id=100$' "${work}/output" || { printf 'FAIL: version-id\n' >&2; failures=$((failures + 1)); }
grep -q '^outcome=submitted$' "${work}/output" || { printf 'FAIL: outcome\n' >&2; failures=$((failures + 1)); }
grep -q '^version-status=TAKEN_FOR_MODERATION$' "${work}/output" || { printf 'FAIL: version-status\n' >&2; failures=$((failures + 1)); }
grep -q '^submitted=true$' "${work}/output" || { printf 'FAIL: submitted\n' >&2; failures=$((failures + 1)); }
grep -q '^skipped=false$' "${work}/output" || { printf 'FAIL: skipped\n' >&2; failures=$((failures + 1)); }
grep -q '^error-code=$' "${work}/output" || { printf 'FAIL: error-code\n' >&2; failures=$((failures + 1)); }
grep -q '^result-file=' "${work}/output" || { printf 'FAIL: result-file\n' >&2; failures=$((failures + 1)); }
grep -q 'Нет списка изменений' "${work}/summary" || { printf 'FAIL: предупреждение в сводке\n' >&2; failures=$((failures + 1)); }
check grep -q -- '--dry-run' "$args_file"
check grep -q -- '--yes' "$args_file"
check grep -q -- '--metadata-dir' "$args_file"
check grep -q -- '--version-code' "$args_file"
check grep -q -- '--api-url' "$args_file"
check grep -q '^https://api.example.test$' "$args_file"
check grep -q -- '--no-embedded-ca' "$args_file"
check grep -q '^--publish-type$' "$args_file"
check grep -q '^--if-exists$' "$args_file"
check test "$(grep -A1 '^--credentials$' "$args_file" | tail -1 | wc -c)" -gt 1

# Ошибка версии: код CLI и error-code сохраняются.
cat >"$result_file" <<'EOF'
{"schema_version":1,"command":"publish","package":"com.example.app","version_code":42,"version_id":null,
 "phase":"create","outcome":"failed","version_status":null,"testing_type":null,"submitted":null,
 "published":null,"skipped":false,"manual_action":false,"outcome_unknown":false,"warnings":[],
 "error":{"code":"VERSION_EXISTS","message":"уже отправлена","hint":"используйте skip"}}
EOF
: >"${work}/output"
: >"${work}/summary"
set +e
FAKE_EXIT_CODE=5 run_publish
code=$?
set -e
if [ "$code" -ne 5 ]; then
	printf 'FAIL: код завершения %d, ожидался 5\n' "$code" >&2
	failures=$((failures + 1))
fi
grep -q '^error-code=VERSION_EXISTS$' "${work}/output" || { printf 'FAIL: error-code ошибки\n' >&2; failures=$((failures + 1)); }
grep -q '^outcome=failed$' "${work}/output" || { printf 'FAIL: outcome ошибки\n' >&2; failures=$((failures + 1)); }

# Код 8: требуется действие в консоли.
cat >"$result_file" <<'EOF'
{"schema_version":1,"command":"publish","package":"com.example.app","version_code":42,"version_id":100,
 "phase":"upload","outcome":"failed","version_status":"DRAFT","testing_type":"release","submitted":false,
 "published":false,"skipped":false,"manual_action":true,"outcome_unknown":false,"warnings":[],
 "error":{"code":"AAB_SIGNING_REQUIRED","message":"нужна подпись","hint":"настройте подпись"}}
EOF
: >"${work}/output"
: >"${work}/summary"
set +e
FAKE_EXIT_CODE=8 run_publish
code=$?
set -e
[ "$code" -eq 8 ] || { printf 'FAIL: код 8 не сохранён\n' >&2; failures=$((failures + 1)); }
grep -q '^manual-action=true$' "${work}/output" || { printf 'FAIL: manual-action\n' >&2; failures=$((failures + 1)); }
grep -q 'Требуется действие в консоли RuStore' "${work}/summary" || { printf 'FAIL: подсказка ручного действия\n' >&2; failures=$((failures + 1)); }

# Неизвестный результат записи.
cat >"$result_file" <<'EOF'
{"schema_version":1,"command":"publish","package":"com.example.app","version_code":42,"version_id":100,
 "phase":"upload","outcome":"unknown","version_status":"DRAFT","testing_type":"release","submitted":null,
 "published":null,"skipped":false,"manual_action":false,"outcome_unknown":true,"warnings":[],
 "error":{"code":"OUTCOME_UNKNOWN","message":"неизвестно","hint":"проверьте status"}}
EOF
: >"${work}/output"
: >"${work}/summary"
set +e
FAKE_EXIT_CODE=6 run_publish
set -e
grep -q '^outcome-unknown=true$' "${work}/output" || { printf 'FAIL: outcome-unknown\n' >&2; failures=$((failures + 1)); }
grep -q 'Результат не подтверждён' "${work}/summary" || { printf 'FAIL: сводка неизвестного результата\n' >&2; failures=$((failures + 1)); }

# Взаимоисключающие учётные данные.
if RUSTORE_TEST_ONLY_BINARY="${work}/rustore-publisher" RUSTORE_VERSION=0.0.1 \
	RUSTORE_AAB="${work}/app.aab" RUSTORE_CREDENTIALS_FILE="${work}/creds.json" \
	RUSTORE_CREDENTIALS_BASE64=e30= bash "$publish" >/dev/null 2>&1; then
	printf 'FAIL: обе формы учётных данных приняты\n' >&2
	failures=$((failures + 1))
fi

# Base64-учётные данные декодируются во временный файл.
cat >"$result_file" <<'EOF'
{"schema_version":1,"command":"publish","package":"com.example.app","version_code":42,"version_id":100,
 "phase":"status","outcome":"submitted","version_status":"ACTIVE","testing_type":"release","submitted":true,
 "published":true,"skipped":false,"manual_action":false,"outcome_unknown":false,"warnings":[],"error":null}
EOF
credentials_json='{"keyId":"key-1","privateKey":"secret"}'
credentials_base64="$(printf '%s' "$credentials_json" | base64 | tr -d '\n')"
: >"${work}/output"
if ! FAKE_CREDENTIALS_EXPECT='key-1' \
	FAKE_RESULT_FILE="$result_file" FAKE_ARGS_FILE="$args_file" \
	RUSTORE_TEST_ONLY_BINARY="${work}/rustore-publisher" RUSTORE_VERSION=0.0.1 \
	RUSTORE_AAB="${work}/app.aab" RUSTORE_CREDENTIALS_BASE64="$credentials_base64" \
	GITHUB_OUTPUT="${work}/output" GITHUB_STEP_SUMMARY="${work}/summary" \
	bash "$publish"; then
	printf 'FAIL: credentials-json-base64 не обработан\n' >&2
	failures=$((failures + 1))
fi

# Без version и без тега выпуска — ошибка.
if RUSTORE_TEST_ONLY_BINARY="${work}/rustore-publisher" RUSTORE_AAB="${work}/app.aab" \
	bash "$publish" >/dev/null 2>&1; then
	printf 'FAIL: отсутствие версии не обнаружено\n' >&2
	failures=$((failures + 1))
fi

# Версия берётся из тега действия.
if ! RUSTORE_ACTION_REF=v0.0.1 RUSTORE_TEST_ONLY_BINARY="${work}/rustore-publisher" \
	RUSTORE_AAB="${work}/app.aab" FAKE_RESULT_FILE="$result_file" \
	GITHUB_OUTPUT="${work}/output" GITHUB_STEP_SUMMARY="${work}/summary" bash "$publish"; then
	printf 'FAIL: версия из тега действия не принята\n' >&2
	failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
	printf 'Провалено проверок: %d\n' "$failures" >&2
	exit 1
fi
printf 'publish_test: ok\n'
