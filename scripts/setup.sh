#!/usr/bin/env bash
# Устанавливает выпущенный CLI rustore-publisher для Linux.
#
# Используется действиями setup и publish. Скачивает архив платформы,
# сверяет SHA-256 с checksums.txt и добавляет каталог в PATH.
#
# Тестовые переопределения (не входы действия):
#   RUSTORE_TEST_ONLY_OS, RUSTORE_TEST_ONLY_ARCH  подменить платформу
#   RUSTORE_INSTALL_DIR                          каталог установки
#   RUSTORE_RELEASES_BASE_URL                    базовый URL выпусков
#   RUSTORE_REPOSITORY                           репозиторий выпусков
set -euo pipefail

readonly DEFAULT_REPOSITORY="vyachin/rustore-publisher"

die() {
	printf 'rustore-publisher setup: %s\n' "$*" >&2
	exit 1
}

log() {
	printf 'rustore-publisher setup: %s\n' "$*" >&2
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	elif command -v openssl >/dev/null 2>&1; then
		openssl dgst -sha256 "$1" | awk '{print $NF}'
	else
		die "не найден инструмент SHA-256 (нужен sha256sum, shasum или openssl)"
	fi
}

raw_version="${RUSTORE_VERSION:-}"
[ -n "$raw_version" ] || die "не задана версия CLI (X.Y.Z или vX.Y.Z)"

version="${raw_version#v}"
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
	die "некорректная версия \"$raw_version\": ожидается X.Y.Z или vX.Y.Z"
fi

os="${RUSTORE_TEST_ONLY_OS:-$(uname -s)}"
arch="${RUSTORE_TEST_ONLY_ARCH:-$(uname -m)}"

case "$os" in
	Linux) ;;
	*) die "неподдерживаемая ОС \"$os\": действия работают только на Linux" ;;
esac

case "$arch" in
	x86_64|amd64) arch="amd64" ;;
	aarch64|arm64) arch="arm64" ;;
	*) die "неподдерживаемая архитектура \"$arch\": поддерживаются x86_64 и aarch64" ;;
esac

asset="rustore-publisher_${version}_linux_${arch}.tar.gz"
repository="${RUSTORE_REPOSITORY:-$DEFAULT_REPOSITORY}"
base_url="${RUSTORE_RELEASES_BASE_URL:-https://github.com/${repository}/releases/download}"
base_url="${base_url%/}/v${version}"

tmp_root="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
install_dir="${RUSTORE_INSTALL_DIR:-${tmp_root}/rustore-publisher/${version}}"

work_dir="$(mktemp -d "${tmp_root%/}/rustore-setup.XXXXXX")"
cleanup() {
	rm -rf "$work_dir"
}
trap cleanup EXIT INT TERM

mkdir -p "$install_dir"

log "установка rustore-publisher ${version} для linux/${arch}"

download() {
	curl --fail --silent --show-error --location --retry 3 --retry-delay 1 \
		--output "$2" "$1" || die "не удалось скачать $1"
}

download "${base_url}/${asset}" "${work_dir}/${asset}"
download "${base_url}/checksums.txt" "${work_dir}/checksums.txt"

expected="$(awk -v name="$asset" '$2 == name { print tolower($1); exit }' "${work_dir}/checksums.txt")"
[ -n "$expected" ] || die "в checksums.txt нет ${asset}"
case "$expected" in
	*[!0-9a-f]*) die "некорректная контрольная сумма для ${asset}" ;;
esac
[ "${#expected}" -eq 64 ] || die "некорректная контрольная сумма для ${asset}"

actual="$(sha256_file "${work_dir}/${asset}" | tr '[:upper:]' '[:lower:]')"
if [ "$actual" != "$expected" ]; then
	die "несовпадение SHA-256 для ${asset}: получено ${actual}, ожидалось ${expected}"
fi

tar -xzf "${work_dir}/${asset}" -C "$install_dir"
binary="${install_dir}/rustore-publisher"
[ -f "$binary" ] || die "в архиве нет rustore-publisher"
chmod 0755 "$binary"

if [ -n "${GITHUB_PATH:-}" ]; then
	printf '%s\n' "$install_dir" >>"$GITHUB_PATH"
fi

if [ -n "${GITHUB_OUTPUT:-}" ]; then
	{
		printf 'version=%s\n' "$version"
		printf 'path=%s\n' "$install_dir"
	} >>"$GITHUB_OUTPUT"
fi

printf '%s\n' "$install_dir"
