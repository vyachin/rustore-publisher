# rustore-publisher

[![CI](https://github.com/vyachin/rustore-publisher/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/vyachin/rustore-publisher/actions/workflows/ci.yml)
[![Релиз](https://img.shields.io/github/v/release/vyachin/rustore-publisher?sort=semver&style=flat-square)](https://github.com/vyachin/rustore-publisher/releases/latest)
[![Версия Go](https://img.shields.io/github/go-mod/go-version/vyachin/rustore-publisher)](go.mod)
[![Лицензия](https://img.shields.io/github/license/vyachin/rustore-publisher)](LICENSE)

**Отправляйте обновления Android-приложения в RuStore из терминала или CI.**
`rustore-publisher` принимает готовый подписанный Android App Bundle (AAB),
создаёт черновик версии, загружает сборку и отправляет её на модерацию.
Список изменений и контакты разработчика можно хранить рядом с кодом.

```bash
rustore-publisher publish -f app-release.aab --metadata-dir metadata
```

Инструмент берёт на себя загрузку очередного обновления после сборки приложения.
Один бинарный файл подходит для локального запуска и CI; для GitHub Actions
есть готовые действия установки и публикации.

Проект дополняет [google-play-publisher](https://github.com/vyachin/google-play-publisher):
два инструмента позволяют отправлять обновления в Google Play и RuStore,
используя общую папку `metadata` со списком изменений.
[Пример совместного использования](#одна-папка-metadata-для-google-play-и-rustore).

По умолчанию после одобрения модерации вы выпускаете версию вручную в консоли
RuStore. Если нужен автоматический выпуск, укажите `--publish-type instantly`.
Команда дожидается отправки на модерацию и завершается, не ожидая её решения.

## Когда использовать

| Задача | Как решается |
| --- | --- |
| Отправить собранное обновление с компьютера | `publish` загружает AAB и отправляет версию на модерацию |
| Проверить доступ и сборку перед отправкой | `doctor` проверяет доступ, `publish --dry-run` — готовность к отправке без записи |
| Узнать, что происходит с отправленной версией | `status` показывает состояние и комментарий модерации |
| Добавить отправку в процесс выпуска | Действие GitHub Actions вызывает тот же CLI |
| Использовать CLI в другом CI или скрипте | Флаг `--json` даёт структурированный результат и коды ошибок |

Проект рассчитан на **обновление уже опубликованного приложения**: в RuStore
должна быть активная версия и заранее настроенная подпись AAB. Сборку и подпись
выполняет ваш Android-проект. APK, первичная настройка приложения, загрузка
иконок и скриншотов, изменение описания карточки и частичный охват
не поддерживаются.

## Быстрый старт

### 1. Установите CLI

Скачайте архив для своей платформы из
[выпуска v0.0.2](https://github.com/vyachin/rustore-publisher/releases/tag/v0.0.2),
сверьте его с `checksums.txt`, распакуйте и добавьте каталог с бинарным файлом
в `PATH`. Готовые сборки есть для Linux, macOS и Windows на amd64 и arm64;
Go для их запуска не нужен. [Пример установки из архива](docs/actions.md#установка-cli).

Если установлен Go версии, указанной в [go.mod](go.mod), можно собрать CLI:

```bash
go install github.com/vyachin/rustore-publisher/cmd/rustore-publisher@v0.0.2
```

Каталог установки Go (по умолчанию `$HOME/go/bin`) должен быть в `PATH`.
Проверьте запуск:

```bash
rustore-publisher version
```

### 2. Настройте доступ к RuStore

Подготовьте ключ API с доступом к приложению и сохраните учётные данные
в JSON-файле, например `$HOME/.config/rustore/credentials.json`:

```json
{
  "keyId": "KEY_ID",
  "privateKey": "BASE64_PKCS8_RSA_PRIVATE_KEY"
}
```

`privateKey` — приватный ключ RSA в Base64, формат DER PKCS#8.
Укажите путь к файлу:

```bash
chmod 600 "$HOME/.config/rustore/credentials.json"
export RUSTORE_CREDENTIALS_FILE="$HOME/.config/rustore/credentials.json"
```

Можно передать путь через `--credentials`; флаг имеет приоритет над переменной
окружения. Не добавляйте файл в Git. Токены хранятся только в памяти
и не выводятся в логах.

По умолчанию CLI обращается к `https://public-api-m.rustore.ru`.
Сертификаты Минцифры встроены в бинарный файл, устанавливать их в систему
не требуется. [Источники и отпечатки сертификатов](internal/rustore/certs/README.md).

### 3. Проверьте сборку и отправьте обновление

Замените `com.example.app` на пакет своего приложения, а `app-release.aab` —
на путь к готовой подписанной сборке. Её `versionCode` должен быть больше
кода активной версии.

```bash
# Проверить доступ к приложению и локальный AAB.
rustore-publisher doctor -p com.example.app -f app-release.aab

# Проверить готовность к отправке без изменений в магазине.
rustore-publisher publish -f app-release.aab --dry-run

# Создать черновик, загрузить AAB и отправить на модерацию.
rustore-publisher publish -f app-release.aab

# Позже проверить состояние версии.
rustore-publisher status -p com.example.app
```

Пакет и код версии для `publish` читаются из AAB. Флаги `--package`
и `--version-code` позволяют дополнительно проверить ожидаемые значения.
Без каталога метаданных новая версия наследует список изменений и контакты
активной версии; CLI предупреждает о наследовании списка изменений.

## Сценарии использования

### Отправить обновление со своим списком изменений

Для AAB с `versionCode=42` подготовьте файл `metadata/ru-RU/changelogs/42.txt`:

```text
metadata/
└── ru-RU/
    └── changelogs/
        └── 42.txt
```

Запишите в него текст для пользователей, например «Исправлена загрузка профиля».
Файл должен быть непустым, в UTF-8, до 5000 символов. Имя файла соответствует
Android `versionCode`, а не `versionName`.

```bash
rustore-publisher publish -f app-release.aab --metadata-dir metadata --dry-run
rustore-publisher publish -f app-release.aab --metadata-dir metadata
```

При переданном `--metadata-dir` файл для кода текущей сборки обязателен.
Название, описания и `images/` из каталога другого магазина не загружаются:
CLI перечисляет их в предупреждении `METADATA_IGNORED`.

Чтобы обновить и контакты разработчика, добавьте
`metadata/developer-contacts.json`:

```json
{
  "email": "dev@example.com",
  "website": "https://example.com"
}
```

Достаточно одного поля. Другой путь можно задать через `--contacts-file`,
в том числе без `--metadata-dir`. [Полный формат и правила проверки контактов](docs/developer-contacts.md).
Остальные поля карточки наследуются из активной версии.

### Одна папка metadata для Google Play и RuStore

[google-play-publisher](https://github.com/vyachin/google-play-publisher)
и [rustore-publisher](https://github.com/vyachin/rustore-publisher)
дополняют друг друга в процессе выпуска Android-приложения. Подготовьте список
изменений один раз, сохраните его в Git и передайте одну папку `metadata`
обоим инструментам:

```text
metadata/
├── ru-RU/
│   ├── changelogs/
│   │   └── 42.txt              # Общий список изменений для обоих магазинов
│   ├── title.txt               # Название для Google Play
│   ├── short_description.txt   # Краткое описание для Google Play
│   ├── full_description.txt    # Полное описание для Google Play
│   └── images/                 # Изображения для Google Play
└── developer-contacts.json     # Контакты для RuStore, необязательно
```

Минимальный общий каталог может содержать только `ru-RU/changelogs/42.txt`.
Если сборки для обоих магазинов имеют `versionCode=42`, они используют этот
же файл. Для общего текста учитывайте более строгий лимит Google Play —
до 500 символов; RuStore допускает до 5000.

```bash
# Учётные данные каждого магазина настраиваются отдельно.

# Отправить сборку во внутреннее тестирование Google Play.
google-play-publisher publish -f app-play-release.aab --metadata-dir metadata

# Отправить сборку на модерацию RuStore.
rustore-publisher publish -f app-rustore-release.aab --metadata-dir metadata
```

Можно использовать отдельные AAB для магазинов, например с разными SDK,
сохраняя общий текст изменений. При разных `versionCode` добавьте файл
для каждого кода версии в тот же каталог `changelogs/`.

Google Play использует локализованные тексты и изображения своей карточки.
RuStore читает `ru-RU/changelogs/<versionCode>.txt` и необязательные контакты;
остальные локали и файлы карточки пропускает с `METADATA_IGNORED`.
Каждая команда выполняется отдельно: отправка в один магазин не запускает
отправку в другой, а модерация и выпуск проходят по правилам каждого магазина.

### Выпустить версию автоматически после модерации

```bash
rustore-publisher publish -f app-release.aab \
  --metadata-dir metadata --publish-type instantly
```

| Режим | Что происходит после одобрения |
| --- | --- |
| `manual` — по умолчанию | Вы выпускаете версию вручную в консоли RuStore |
| `instantly` | RuStore публикует версию автоматически |

Оба режима отправляют версию на модерацию. Отдельной команды ручного выпуска
в CLI нет.

### Проверить отправленную версию

```bash
# Показать последний выпуск и существующий черновик.
rustore-publisher status -p com.example.app

# Прочитать конкретную версию по идентификатору RuStore.
rustore-publisher status -p com.example.app --version-id 123456
```

В выводе доступны статус, код версии, режим публикации и комментарий модерации.
`--version-id` — идентификатор RuStore, он отличается от Android `versionCode`.

### Повторить запуск после сбоя CI

```bash
rustore-publisher publish -f app-release.aab --if-exists skip
```

Если версия с таким кодом уже отправлена, команда завершится успешно
с признаком `skipped=true`. По умолчанию используется `--if-exists fail`,
и повторная отправка считается конфликтом.

Существующий черновик останавливает отправку даже с `skip`: завершите
или удалите его в консоли RuStore. CLI не заменяет черновики.
Если результат записи неизвестен (`outcome_unknown=true`), сначала проверьте
версию через `status` или консоль, прежде чем повторять отправку.

### Отправлять обновления из GitHub Actions

Добавьте шаг публикации после сборки и подписи AAB. Ниже — фрагмент workflow;
в отмеченном месте вставьте шаги сборки своего Android-проекта или загрузки
готового артефакта:

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
      # Здесь соберите и подпишите AAB или скачайте готовую сборку.
      - uses: vyachin/rustore-publisher@v0.0.2
        with:
          version: 0.0.2
          package: com.example.app
          aab: app/build/outputs/bundle/release/app-release.aab
          credentials-json-base64: ${{ secrets.RUSTORE_CREDENTIALS_BASE64 }}
          metadata-dir: metadata
          publish-type: manual
          if-exists: skip
```

В секрете `RUSTORE_CREDENTIALS_BASE64` сохраните Base64 **всего JSON-файла**
учётных данных. Это дополнительное кодирование файла, отдельное от Base64
приватного ключа внутри него. Подготовить значение можно так:

```bash
# Linux
base64 -w0 credentials.json

# macOS
base64 -i credentials.json | tr -d '\n'
```

Действия работают на Linux amd64/arm64. Для установки CLI без публикации
используйте `vyachin/rustore-publisher/setup@v0.0.2` с `version: 0.0.2`.
[Все входы, выходы и правила восстановления](docs/actions.md).

### Получить результат для скрипта или другого CI

```bash
rustore-publisher status -p com.example.app --json > status.json
rustore-publisher publish -f app-release.aab --json > publish-result.json
```

С `--json` в stdout выводится ровно один объект, диагностика идёт в stderr.
Скрипт может проверить `outcome`, `version_id`, `submitted`, `skipped`
и `error.code`. Успешная отправка на модерацию не означает, что версия уже
доступна пользователям. [Схема результата](docs/schema/result.schema.json)
и [примеры JSON](docs/schema/examples).

Коды завершения: `0` — успех, `1` — внутренняя ошибка, `2` — неверный ввод,
`3` — авторизация или доступ, `4` — объект не найден, `5` — конфликт,
`6` — временная ошибка или неизвестный результат, `7` — отклонение,
`8` — требуется действие в консоли, `130` — прерывание.

## Справка и документация

```bash
rustore-publisher --help
rustore-publisher publish --help
rustore-publisher completion bash
```

Все команды: `doctor`, `status`, `publish`, `version`, `completion`.
Общий таймаут команды по умолчанию — 30 минут (`--timeout`), отдельного
запроса — 2 минуты (`--request-timeout`), ожидания обработки AAB — 2 минуты
(`--processing-timeout`).

- [Контракт CLI, API и ошибок](docs/design.md).
- [Настройка GitHub Actions](docs/actions.md).
- [Контакты разработчика](docs/developer-contacts.md).
- [История версий](CHANGELOG.md).
- [План и состояние работ](TODO.md).
- [Архитектурные решения](docs/adr).

В версии 0.0.2 чтения и отправка на модерацию проверены на живом приложении.
Публикация после одобрения модерации ещё не подтверждена.

## Разработка

Проект написан на Go с Cobra. Для локальной сборки и проверки:

```bash
make build
make test
```

Бинарный файл появится в `bin/rustore-publisher`.
[Правила проекта](AGENTS.md) и [go.mod](go.mod) определяют требования
к изменениям и версии Go. Go-модули и версии GitHub Actions обновляет
Dependabot раз в неделю; конфигурация — [.github/dependabot.yml](.github/dependabot.yml).
Dependabot alerts и security updates включены в настройках репозитория.

Проект разработан при существенной помощи ИИ: DeepSeek Harness (deepseek-flash)
и OpenAI Codex, под руководством и проверкой автора. Автор отвечает
за архитектуру, процесс выпуска и проверку результатов.

## Лицензия

[MIT](LICENSE).
