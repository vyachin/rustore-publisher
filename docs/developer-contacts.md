# Формат файла developer-contacts.json

Файл передаёт контакты разработчика в теле создания черновика RuStore
(`developerContacts`). Контакты — публичные данные карточки приложения,
не секреты; ключи API в этот файл не попадают.

## Расположение и приоритет

1. `--contacts-file PATH` — явно указанный файл, используется всегда.
2. `<metadata-dir>/developer-contacts.json` — если флаг не задан, но файл есть
   в каталоге метаданных.
3. Если ни одного файла нет, поле `developerContacts` не передаётся, и RuStore
   наследует контакты из активной версии приложения.

Флаг `--contacts-file` не требует `--metadata-dir`. Если каталог метаданных
не задан, список изменений тоже не передаётся — CLI предупредит
`CHANGELOG_INHERITED`.

## Структура

- JSON-объект в UTF-8 без BOM, без комментариев.
- Разрешены только поля из таблицы ниже; неизвестное поле — ошибка.
- Все поля необязательные, но хотя бы одно должно быть непустым.
- Пробелы по краям значений обрезаются.
- Файл должен содержать ровно один JSON-объект.

| Поле | Тип | Правило | Пример |
| --- | --- | --- | --- |
| `email` | string | почта вида `name@domain.tld`, без пробелов | `dev@example.com` |
| `website` | string | абсолютный `http(s)`-адрес с хостом | `https://example.com` |
| `vkCommunity` | string | абсолютный `http(s)`-адрес сообщества | `https://vk.com/example` |
| `maxMessenger` | string | абсолютный `http(s)`-адрес в MAX | `https://max.ru/example` |

## Примеры

Минимальный вариант — только почта:

```json
{
  "email": "dev@example.com"
}
```

Только сайт:

```json
{
  "website": "https://example.com"
}
```

Полный вариант:

```json
{
  "email": "dev@example.com",
  "website": "https://example.com",
  "vkCommunity": "https://vk.com/example",
  "maxMessenger": "https://max.ru/example"
}
```

Неверные варианты:

```json
{}
```

```json
{"email": "dev@example.com", "phone": "+7 900 000-00-00"}
```

Первая ошибка — ни одного способа связи, вторая — неизвестное поле `phone`.
Обе отклоняются до обращения к сети с кодом 2 и подсказкой.

## Как это уходит в API

Контакты сериализуются в объект `developerContacts` запроса
`POST /public/v1/application/{package}/version`:

```json
{
  "publishType": "MANUAL",
  "partialValue": 100,
  "whatsNew": "…",
  "developerContacts": {
    "email": "dev@example.com",
    "website": "https://example.com"
  }
}
```

Пустые поля не отправляются. В документации RuStore поле описано как массив,
но пример запроса содержит объект; CLI отправляет объект согласно примеру.

## Ошибки

Все ошибки файла возвращаются до авторизации и обращения к сети:

| Ситуация | `error.code` | Код завершения |
| --- | --- | --- |
| файл не найден или не читается | `INVALID_ARGUMENTS` | 2 |
| некорректный JSON, два объекта, неизвестное поле | `INVALID_ARGUMENTS` | 2 |
| нет ни одного непустого поля | `INVALID_ARGUMENTS` | 2 |
| некорректный email или адрес | `INVALID_ARGUMENTS` | 2 |

Если контакты не переданы, а в активной версии они пусты, RuStore может
отклонить создание черновика с `400 Developer contacts are not filled in`.
CLI вернёт `API_REJECTED` (код 7) и подскажет заполнить поля в консоли.

## Проверка без записи

```bash
rustore-publisher publish -f app-release.aab \
  --contacts-file metadata/developer-contacts.json --dry-run
```

`--dry-run` проверяет файл и состояние магазина, но не создаёт черновик.

## GitHub Actions

Вход `contacts-file` действия публикации:

```yaml
- uses: vyachin/rustore-publisher@v0.0.1
  with:
    version: 0.0.1
    aab: app/build/outputs/bundle/release/app-release.aab
    metadata-dir: metadata
    contacts-file: metadata/developer-contacts.json
    credentials-base64: ${{ secrets.RUSTORE_CREDENTIALS_BASE64 }}
```

Без `contacts-file` действие использует
`<metadata-dir>/developer-contacts.json`, если файл существует.
