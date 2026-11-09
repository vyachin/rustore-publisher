# rustore-publisher

CLI и составные действия GitHub Actions для публикации Android App Bundle
в RuStore.

Статус: версия 0.0.1 в разработке. Все пять команд, архивы выпуска и GitHub
Actions реализованы; остаются проверка на тестовом приложении и выпуск тега
`v0.0.1`. План и чек-лист — [TODO.md](TODO.md), полный контракт —
[docs/design.md](docs/design.md).

## Возможности 0.0.1

- `publish -f app.aab` — локальная проверка AAB и списка изменений, проверка
  состояния приложения, создание черновика с ручной публикацией, потоковая
  загрузка, ожидание обработки и отправка на модерацию.
- `status -p PACKAGE [--version-id ID]` — статус версии, код версии, режим
  публикации, комментарий модерации и живой черновик.
- `doctor -p PACKAGE [-f app.aab]` — авторизация и проверки доступа без изменений
  в магазине.
- `version` и `completion` — сведения о сборке и скрипт автодополнения.

`publish` только отправляет черновик на модерацию. После одобрения версия
выпускается вручную в консоли RuStore; отдельной команды выпуска нет.

## Требования

- Go 1.27 или новее для сборки из исходников.
- Активная версия приложения в RuStore и заранее настроенная подпись AAB.
- Ключ API RuStore: `keyId` и приватный ключ RSA в Base64 (DER PKCS#8).

## Учётные данные

Файл в формате:

```json
{"keyId": "KEY_ID", "privateKey": "BASE64_PKCS8_RSA_PRIVATE_KEY"}
```

Путь задаётся флагом `--credentials` или переменной окружения
`RUSTORE_CREDENTIALS_FILE`; флаг имеет приоритет. Файл не добавляйте в Git,
рекомендуется режим `0600`. Токены хранятся только в памяти и не выводятся
в логах.

## Быстрый старт

```bash
export RUSTORE_CREDENTIALS_FILE="$HOME/.config/rustore/credentials.json"
# По умолчанию используется https://public-api-m.rustore.ru; сертификаты
# Минцифры встроены в бинарник. Старый домен: --api-url
# https://public-api.rustore.ru.
rustore-publisher doctor -p com.example.app -f app-release.aab
rustore-publisher publish -f app-release.aab --metadata-dir metadata
rustore-publisher status -p com.example.app
```

## Сертификаты

Официальные корневой и промежуточный сертификаты Минцифры встроены в бинарный
файл через `go:embed`, поэтому для домена `public-api-m.rustore.ru` не нужно
передавать `SSL_CERT_FILE` или устанавливать CA в систему. Они добавляются
к системному пулу только в HTTP-клиенте RuStore, а редиректы запрещены.
Источники и отпечатки — в
[internal/rustore/certs/README.md](internal/rustore/certs/README.md).

## Список изменений

Файл `metadata/ru-RU/changelogs/<versionCode>.txt` в UTF-8, до 5000 символов.
Без `--metadata-dir` поле `whatsNew` не передаётся, а CLI предупреждает, что
пользователи увидят прежние заметки активной версии. Другие локали и файлы
карточки в 0.0.1 отклоняются.

## Режим публикации

По умолчанию `--publish-type=manual`: после модерации нужен ручной выпуск
в консоли. Режим `instantly` требует явного `--yes`. Отложенная публикация
появится в 0.0.2.

## Коды завершения

0 — успех, 1 — внутренняя ошибка, 2 — ввод, 3 — авторизация или доступ,
4 — объект не найден, 5 — конфликт, 6 — временная ошибка или неизвестный
результат, 7 — отклонение, 8 — требуется действие в консоли, 130 — прерывание.
Полный список `error.code` — в [docs/design.md](docs/design.md).

## JSON

С `--json` CLI печатает ровно один объект в stdout, диагностика идёт в stderr.
Схема результата — [docs/schema/result.schema.json](docs/schema/result.schema.json),
примеры — в [docs/schema/examples](docs/schema/examples).

## GitHub Actions

- `vyachin/rustore-publisher@v0.0.1` — установка CLI и публикация AAB.
- `vyachin/rustore-publisher/setup@v0.0.1` — только установка CLI.

Входы, выходы, секреты и восстановление описаны в
[docs/actions.md](docs/actions.md). Архивы выпуска собираются GoReleaser
для Linux, macOS и Windows на amd64 и arm64; действия работают на Linux.

## Документация

- [docs/design.md](docs/design.md) — контракт CLI, API, JSON и ошибок.
- [docs/adr](docs/adr) — архитектурные решения.
- [TODO.md](TODO.md) — план и состояние работ.
- [AGENTS.md](AGENTS.md) — правила разработки.

## Лицензия

MIT, см. [LICENSE](LICENSE). История версий — [CHANGELOG.md](CHANGELOG.md).
