#!/usr/bin/env bash
# Проверяет установку CLI из локального «выпуска».
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
setup="${script_dir}/../scripts/setup.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

failures=0
check() {
	if ! "$@"; then
		printf 'FAIL: %s\n' "$1" >&2
		failures=$((failures + 1))
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

make_release() {
	local version="$1" root="$2"
	mkdir -p "${root}/v${version}/payload"
	cat >"${root}/v${version}/payload/rustore-publisher" <<'EOF'
#!/usr/bin/env bash
echo "rustore-publisher ${RUSTORE_TEST_VERSION:-0.0.1}"
EOF
	chmod 0755 "${root}/v${version}/payload/rustore-publisher"
	: >"${root}/v${version}/checksums.txt"
	for arch in amd64 arm64; do
		local asset="rustore-publisher_${version}_linux_${arch}.tar.gz"
		tar -czf "${root}/v${version}/${asset}" \
			-C "${root}/v${version}/payload" rustore-publisher
		sha256 "${root}/v${version}/${asset}" |
			awk -v name="$asset" '{print $1 "  " name}' >>"${root}/v${version}/checksums.txt"
	done
}

root="${work}/releases"
make_release 0.0.1 "$root"

run_setup() {
	RUSTORE_VERSION="${1:-0.0.1}" \
		RUSTORE_TEST_ONLY_OS=Linux \
		RUSTORE_TEST_ONLY_ARCH="${2:-x86_64}" \
		RUSTORE_RELEASES_BASE_URL="file://${root}" \
		RUSTORE_INSTALL_DIR="${work}/install" \
		GITHUB_OUTPUT="${work}/output" \
		GITHUB_PATH="${work}/path" \
		bash "$setup"
}

if ! install_dir="$(run_setup 0.0.1 x86_64)"; then
	printf 'FAIL: установка не удалась\n' >&2
	exit 1
fi
check test -x "${install_dir}/rustore-publisher"
check grep -q '^version=0.0.1$' "${work}/output"
check grep -q "^path=${install_dir}$" "${work}/output"
check grep -q "^${install_dir}$" "${work}/path"
check test "$("${install_dir}/rustore-publisher")" = "rustore-publisher 0.0.1"

# Версия с префиксом v и другой архитектурой.
make_release 0.0.2 "$root"
if ! install_dir="$(run_setup v0.0.2 aarch64)"; then
	printf 'FAIL: установка версии с v не удалась\n' >&2
	exit 1
fi
check test -x "${install_dir}/rustore-publisher"

# Несовпадение контрольной суммы.
cp "${root}/v0.0.1/checksums.txt" "${root}/v0.0.1/checksums.bak"
printf '%064d  %s\n' 0 "rustore-publisher_0.0.1_linux_amd64.tar.gz" \
	>"${root}/v0.0.1/checksums.txt"
if run_setup 0.0.1 >/dev/null 2>&1; then
	printf 'FAIL: несовпадение SHA-256 не обнаружено\n' >&2
	failures=$((failures + 1))
fi
mv "${root}/v0.0.1/checksums.bak" "${root}/v0.0.1/checksums.txt"

# Нет нужного файла в checksums.txt.
printf '%064d  other.tar.gz\n' 0 >"${root}/v0.0.1/checksums.txt"
if run_setup 0.0.1 >/dev/null 2>&1; then
	printf 'FAIL: отсутствие файла в checksums.txt не обнаружено\n' >&2
	failures=$((failures + 1))
fi

# Неподдерживаемая ОС.
if RUSTORE_VERSION=0.0.1 RUSTORE_TEST_ONLY_OS=Darwin RUSTORE_RELEASES_BASE_URL="file://${root}" \
	bash "$setup" >/dev/null 2>&1; then
	printf 'FAIL: Darwin должен быть отклонён\n' >&2
	failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
	printf 'Провалено проверок: %d\n' "$failures" >&2
	exit 1
fi
printf 'setup_test: ok\n'
