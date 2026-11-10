# GitHub Actions

Действия вызывают выпущенный CLI и не дублируют логику публикации. Версия CLI
должна быть указана явно; действие не скачивает последнюю версию молча.

## Установка CLI

Скачайте архив нужной платформы из выпуска, сверьте `checksums.txt` и распакуйте:

```bash
version=0.0.3
curl -sSfLO "https://github.com/vyachin/rustore-publisher/releases/download/v${version}/rustore-publisher_${version}_linux_amd64.tar.gz"
curl -sSfLO "https://github.com/vyachin/rustore-publisher/releases/download/v${version}/checksums.txt"
sha256sum --check --ignore-missing checksums.txt
tar -xzf "rustore-publisher_${version}_linux_amd64.tar.gz"
./rustore-publisher version
```

Или соберите из исходников: `go install github.com/vyachin/rustore-publisher/cmd/rustore-publisher@v${version}`.

## Действие setup

```yaml
- uses: vyachin/rustore-publisher/setup@v0.0.3
  with:
    version: 0.0.3
- run: rustore-publisher status -p com.example.app --json
```

Выходы: `version`, `path`. Действие поддерживает Linux amd64/arm64; сборки CLI
для macOS и Windows в выпуске есть, но действия запускаются только на Linux.

## Действие публикации

```yaml
permissions:
  contents: read

concurrency:
  group: rustore-com-example-app
  cancel-in-progress: false

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      # AAB собирается и подписывается здесь или скачивается из задания сборки.
      - uses: vyachin/rustore-publisher@v0.0.3
        with:
          version: 0.0.3
          package: com.example.app
          aab: app/build/outputs/bundle/release/app-release.aab
          credentials-json-base64: ${{ secrets.RUSTORE_CREDENTIALS_BASE64 }}
          metadata-dir: metadata
          # Контакты разработчика: metadata/developer-contacts.json
```

Секрет `RUSTORE_CREDENTIALS_BASE64` — это Base64 от всего JSON-файла учётных
данных:

```bash
base64 -w0 rustore.json   # Linux
base64 -i rustore.json    # macOS, без переносов строк
```

Не храните ключ в репозитории и не передавайте его аргументами CLI.

Сертификаты Минцифры встроены в CLI, поэтому устанавливать CA на runner
не нужно: домен `https://public-api-m.rustore.ru` работает сразу.

### Входы

| Вход | По умолчанию | Значение |
| --- | --- | --- |
| `version` | тег действия | Версия CLI |
| `package` | из AAB | Имя пакета для проверки |
| `aab` | обязателен | Путь к подписанному AAB |
| `credentials-json-base64` | — | Весь JSON учётных данных в Base64 |
| `credentials-file` | — | Путь к файлу учётных данных |
| `metadata-dir` | — | Каталог `metadata` со списком изменений, текстами карточки и контактами |
| `contacts-file` | `metadata/developer-contacts.json` | Файл контактов разработчика, формат — [developer-contacts.md](developer-contacts.md) |
| `version-code` | из AAB | Ожидаемый код версии |
| `publish-type` | `manual` | `manual` или `instantly` |
| `if-exists` | `fail` | `fail` или `skip` для уже отправленной версии |
| `dry-run` | `false` | Только проверка без записи |
| `timeout` | `30m` | Общий бюджет команды |
| `request-timeout` | `2m` | Бюджет одного запроса |
| `processing-timeout` | `2m` | Ожидание обработки AAB |
| `debug` | `false` | Подробный журнал |

### Выходы

`version-code`, `version-id`, `outcome`, `version-status`, `submitted`,
`published`, `skipped`, `manual-action`, `error-code`, `outcome-unknown`,
`phase`, `testing-type`, `result-file`.

Выходы заполняются до возврата ненулевого кода, поэтому шаги после падения могут
прочитать `error-code`, `phase` и `result-file`.

## Повторные запуски и восстановление

- `if-exists: fail` — ошибка 5 `VERSION_EXISTS`, если версия с этим кодом уже
  отправлена. `if-exists: skip` возвращает успех с `skipped=true`.
- Создание может быть отклонено с `API_REJECTED`, если обязательные поля
  карточки пусты (например, контакты разработчика). Заполните их в консоли
  RuStore и повторите; черновик при таком отказе не создаётся.
- Существующий черновик останавливает публикацию с ошибкой `EXISTING_DRAFT`;
  завершите или удалите его в консоли RuStore. CLI не заменяет черновик.
- При `outcome_unknown=true` результат записи не доказан. Проверьте версию
  командой `status` или в консоли RuStore и не создавайте новую версию до
  выяснения.
- После одобрения модерации версия с `publish-type=manual` публикуется вручную
  в консоли RuStore. Действие не ждёт модерацию.
- Не запускайте публикации одного приложения из разных репозиториев и консоли
  одновременно: `concurrency` в GitHub координирует только один репозиторий.

## Ограничения

- Подпись AAB должна быть заранее настроена в консоли RuStore.
- Частичный охват не поддерживается.
- Название и описания карточки читаются из `ru-RU/title.txt`,
  `short_description.txt` и `full_description.txt`. Иконки и скриншоты не
  загружаются; элементы каталога вне поддерживаемых файлов перечисляются
  в предупреждении `METADATA_IGNORED`, а не отклоняются.
