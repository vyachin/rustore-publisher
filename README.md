# rustore-publisher

CLI и составные действия GitHub Actions для публикации Android App Bundle
в RuStore.

Статус: версия 0.0.1 в разработке, команды ещё не реализованы. План и чек-лист —
[TODO.md](TODO.md), полный контракт — [docs/design.md](docs/design.md).

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
rustore-publisher doctor -p com.example.app -f app-release.aab
rustore-publisher publish -f app-release.aab --metadata-dir metadata
rustore-publisher status -p com.example.app
```

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

Корневое действие публикации и `setup` для установки CLI появятся в 0.0.1;
пример workflow описан в [docs/design.md](docs/design.md).

## Документация

- [docs/design.md](docs/design.md) — контракт CLI, API, JSON и ошибок.
- [docs/adr](docs/adr) — архитектурные решения.
- [TODO.md](TODO.md) — план и состояние работ.
- [AGENTS.md](AGENTS.md) — правила разработки.

## Лицензия

MIT, см. [LICENSE](LICENSE).
