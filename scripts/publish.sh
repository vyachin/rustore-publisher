#!/usr/bin/env bash
# Публикует AAB через CLI rustore-publisher.
#
# Используется корневым action.yml. Скрипт готовит учётные данные, собирает
# аргументы, запускает CLI с --json, публикует выходные параметры и сводку.
# Поведение магазина остаётся в CLI.
#
# Тестовое переопределение (не вход действия):
#   RUSTORE_TEST_ONLY_BINARY — использовать указанный исполняемый файл CLI
set -euo pipefail

die() {
	printf 'rustore-publisher publish: %s\n' "$*" >&2
	exit 1
}

warn() {
	printf '::warning::rustore-publisher publish: %s\n' "$*" >&2
}

mask() {
	printf '::add-mask::%s\n' "$1"
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

package="${RUSTORE_PACKAGE:-}"
aab="${RUSTORE_AAB:-}"
publish_type="${RUSTORE_PUBLISH_TYPE:-manual}"
if_exists="${RUSTORE_IF_EXISTS:-fail}"
metadata_dir="${RUSTORE_METADATA_DIR:-}"
contacts_file="${RUSTORE_CONTACTS_FILE:-}"
api_url="${RUSTORE_API_URL:-}"
version_code="${RUSTORE_VERSION_CODE:-}"
timeout_value="${RUSTORE_TIMEOUT:-}"
request_timeout="${RUSTORE_REQUEST_TIMEOUT:-}"
processing_timeout="${RUSTORE_PROCESSING_TIMEOUT:-}"
debug="${RUSTORE_DEBUG:-false}"
dry_run="${RUSTORE_DRY_RUN:-false}"

[ -n "$aab" ] || die "не задан вход aab"

version="${RUSTORE_VERSION:-}"
if [ -z "$version" ]; then
	ref="${RUSTORE_ACTION_REF:-}"
	case "$ref" in
		v[0-9]*|[0-9]*) version="$ref" ;;
		*) die "укажите version: действие запущено не с тега выпуска (${ref})" ;;
	esac
fi

resolve_path() {
	case "$1" in
		/*) printf '%s' "$1" ;;
		*) printf '%s/%s' "${GITHUB_WORKSPACE:-$PWD}" "$1" ;;
	esac
}

tmp_root="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
result_dir="$(mktemp -d "${tmp_root%/}/rustore-publisher.XXXXXX")"
result_file="${result_dir}/result.json"
stderr_file=""
cred_path=""
cred_created=0

# shellcheck disable=SC2329 # вызывается из trap EXIT
cleanup() {
	if [ "$cred_created" -eq 1 ] && [ -n "$cred_path" ]; then
		rm -f "$cred_path"
	fi
	if [ -n "$stderr_file" ]; then
		rm -f "$stderr_file"
	fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -n "${RUSTORE_TEST_ONLY_BINARY:-}" ]; then
	binary="$RUSTORE_TEST_ONLY_BINARY"
	[ -x "$binary" ] || die "тестовый бинарный файл ${binary} не исполняемый"
else
	install_dir="$(GITHUB_OUTPUT='' RUSTORE_VERSION="$version" bash "${script_dir}/setup.sh")" ||
		die "не удалось установить rustore-publisher ${version}"
	binary="${install_dir}/rustore-publisher"
	export PATH="${install_dir}:${PATH}"
fi

base64_value="${RUSTORE_CREDENTIALS_BASE64:-}"
credentials_file="${RUSTORE_CREDENTIALS_FILE:-}"
if [ -n "$base64_value" ] && [ -n "$credentials_file" ]; then
	die "credentials-json-base64 и credentials-file взаимоисключающие"
fi

if [ -n "$base64_value" ]; then
	mask "$(printf '%s' "$base64_value" | tr -d '\r\n')"
	cred_path="${result_dir}/credentials-$$.json"
	cred_created=1
	if ! (umask 077; printf '%s' "$base64_value" | (base64 --decode 2>/dev/null || base64 -D) >"$cred_path"); then
		die "не удалось декодировать credentials-json-base64"
	fi
	[ -s "$cred_path" ] || die "credentials-json-base64 пуст или содержит неверный Base64"
elif [ -n "$credentials_file" ]; then
	cred_path="$(resolve_path "$credentials_file")"
	[ -f "$cred_path" ] || die "файл credentials-file ${credentials_file} не найден"
fi

args=(publish --json --publish-type "$publish_type" --if-exists "$if_exists" --aab "$(resolve_path "$aab")")
if [ -n "$package" ]; then
	args+=(--package "$package")
fi
if [ -n "$api_url" ]; then
	args+=(--api-url "$api_url")
fi
if [ -n "$metadata_dir" ]; then
	args+=(--metadata-dir "$(resolve_path "$metadata_dir")")
fi
if [ -n "$contacts_file" ]; then
	args+=(--contacts-file "$(resolve_path "$contacts_file")")
fi
if [ -n "$version_code" ]; then
	args+=(--version-code "$version_code")
fi
if [ -n "$timeout_value" ]; then
	args+=(--timeout "$timeout_value")
fi
if [ -n "$request_timeout" ]; then
	args+=(--request-timeout "$request_timeout")
fi
if [ -n "$processing_timeout" ]; then
	args+=(--processing-timeout "$processing_timeout")
fi
if [ "$dry_run" = "true" ]; then
	args+=(--dry-run)
fi
if [ "$debug" = "true" ]; then
	args+=(--debug)
fi
if [ -n "$cred_path" ]; then
	args+=(--credentials "$cred_path")
fi

stderr_file="$(mktemp "${tmp_root%/}/rustore-publish-stderr.XXXXXX")"
set +e
"$binary" "${args[@]}" >"$result_file" 2>"$stderr_file"
code=$?
set -e

cat "$stderr_file" >&2
rm -f "$stderr_file"
stderr_file=""

emit() {
	if [ -n "${GITHUB_OUTPUT:-}" ]; then
		printf '%s=%s\n' "$1" "$2" >>"$GITHUB_OUTPUT"
	fi
}

have_jq=0
if command -v jq >/dev/null 2>&1; then
	have_jq=1
else
	warn "jq не установлен: выходные параметры JSON недоступны, файл результата сохранён"
fi

json_field() {
	if [ "$have_jq" -eq 1 ] && [ -s "$result_file" ]; then
		jq -r "(${1}) as \$v | if \$v == null then \"\" else (\$v | tostring) end" "$result_file" 2>/dev/null || true
	fi
}

reported_code="$(json_field '.version_code')"
version_id="$(json_field '.version_id')"
outcome="$(json_field '.outcome')"
version_status="$(json_field '.version_status')"
submitted="$(json_field '.submitted')"
published="$(json_field '.published')"
skipped="$(json_field '.skipped')"
manual_action="$(json_field '.manual_action')"
error_code="$(json_field '.error.code')"
outcome_unknown="$(json_field '.outcome_unknown')"
phase="$(json_field '.phase')"
testing_type="$(json_field '.testing_type')"

[ -n "$skipped" ] || skipped="false"
[ -n "$manual_action" ] || manual_action="false"
[ -n "$outcome_unknown" ] || outcome_unknown="false"

emit "version-code" "$reported_code"
emit "version-id" "$version_id"
emit "outcome" "$outcome"
emit "version-status" "$version_status"
emit "submitted" "$submitted"
emit "published" "$published"
emit "skipped" "$skipped"
emit "manual-action" "$manual_action"
emit "error-code" "$error_code"
emit "outcome-unknown" "$outcome_unknown"
emit "phase" "$phase"
emit "testing-type" "$testing_type"
emit "result-file" "$result_file"

summary="${GITHUB_STEP_SUMMARY:-}"
if [ -n "$summary" ] && [ -n "$outcome" ]; then
	summary_lines=(
		"### Публикация в RuStore"
		""
		"| Поле | Значение |"
		"| --- | --- |"
		"| Пакет | ${package:-из AAB} |"
		"| Версия | ${reported_code:-неизвестно} (id ${version_id:-неизвестно}) |"
		"| Итог | ${outcome} |"
		"| Статус | ${version_status:-неизвестно} |"
		"| Фаза | ${phase:-неизвестно} |"
		"| Код завершения | ${code} |"
	)
	if [ "$manual_action" = "true" ]; then
		summary_lines+=(
			""
			"> **Требуется действие в консоли RuStore.** "
			"Исправьте настройки по подсказке CLI и повторите публикацию."
		)
	fi
	if [ "$outcome_unknown" = "true" ]; then
		summary_lines+=(
			""
			"> **Результат не подтверждён.** Проверьте версию командой status "
			"и в консоли RuStore до повторного запуска."
		)
	fi
	if [ "$have_jq" -eq 1 ] && [ -s "$result_file" ]; then
		while IFS= read -r warning; do
			[ -n "$warning" ] || continue
			summary_lines+=("" "- Предупреждение: ${warning}")
		done < <(jq -r '.warnings[]?.message' "$result_file" 2>/dev/null || true)
	fi
	if ! printf '%s\n' "${summary_lines[@]}" >>"$summary"; then
		warn "не удалось записать сводку задания"
	fi
fi

exit "$code"
