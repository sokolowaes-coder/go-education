# go-education

Учебный REST API на Go для работы с записями: `net/http` + PostgreSQL (`pgx`).

## Запуск

Всё в Docker — база и API (http://localhost:8080):

```sh
docker compose up -d --build   # собрать и запустить
docker compose logs -f app     # логи сервера
docker compose down            # остановить (данные БД сохраняются)
docker compose down -v         # остановить и удалить данные
```

Если порт 8080 занят: `APP_PORT=8081 docker compose up -d`.

Для разработки — только база в Docker, сервер через `go run`:

```sh
docker compose up -d db
DATABASE_URL="postgres://app:app@localhost:5433/go_education?sslmode=disable" go run ./cmd/server
```

Переменные окружения сервера:

| Переменная     | По умолчанию | Описание                                  |
|----------------|--------------|-------------------------------------------|
| `DATABASE_URL` | —            | строка подключения к Postgres (обязательна) |
| `PORT`         | `8080`       | порт HTTP-сервера                         |
| `LOG_LEVEL`    | `info`       | `debug`, `info`, `warn`, `error`          |
| `LOG_FORMAT`   | текст        | `json` — логи в JSON                      |
| `APP_TZ`       | системный    | часовой пояс для дат в фильтрах, напр. `Europe/Moscow` |
| `DATABASE_URL_UNPOOLED` | —   | прямое подключение для миграций, если `DATABASE_URL` идёт через пулер |

По Ctrl+C / `docker stop` сервер дожидается текущих запросов и завершается корректно.

## API

| Метод    | Путь            | Тело               | Ответ                        |
|----------|-----------------|--------------------|------------------------------|
| `GET`    | `/records`      | —                  | `200` список с фильтрами (см. ниже) |
| `POST`   | `/records`      | `{"name":"...", "description":"..."}` | `201` созданная запись |
| `GET`    | `/records/{id}` | —                  | `200` запись                 |
| `PUT`    | `/records/{id}` | `{"name":"...", "description":"..."}` | `200` обновлённая запись |
| `DELETE` | `/records/{id}` | —                  | `204` без тела               |
| `GET`    | `/healthz`      | —                  | `200` `{"status":"ok"}` / `503`, если БД недоступна |

`name` обязателен, `description` — нет (по умолчанию пустая строка). `PUT` заменяет оба поля.

Ошибки: `400` — неверный id, JSON, фильтр или пустое имя; `404` — записи нет; `500` — ошибка сервера.
Тело ошибки всегда JSON: `{"success": false, "error": "запись не найдена"}`.

### Список: фильтры и пагинация

`GET /records` принимает query-параметры, все необязательные:

| Параметр         | Пример                          | Что делает                                      |
|------------------|---------------------------------|-------------------------------------------------|
| `fullText`       | `лазер`                         | поиск подстроки без учёта регистра              |
| `fullTextFields` | `name` или `name,description`   | где искать (по умолчанию — везде)               |
| `id`             | `1,2,3` или `id=1&id=2`         | только записи с этими id                        |
| `dateStart`      | `2026-09-27` / `2026-09-27T10:00:00` | `created_at` не раньше                     |
| `dateEnd`        | `2026-09-27`                    | `created_at` не позже; дата без времени — до конца дня |
| `limit`          | `20`                            | размер страницы, 1–100 (по умолчанию 20)        |
| `offset`         | `0`                             | сколько записей пропустить                      |

Даты без пояса считаются в часовом поясе `APP_TZ` (в Docker — `Europe/Moscow`); можно передать и с поясом: `2026-09-27T10:00:00+03:00`.

```sh
curl 'localhost:8080/records?fullText=лазер&dateStart=2026-09-27&dateEnd=2026-09-27&limit=10'
```

```json
{
  "success": true,
  "filters": {
    "fullText":  {"value": "лазер", "fields": null},
    "id":        {"value": null},
    "dateStart": {"value": "2026-09-27T00:00:00"},
    "dateEnd":   {"value": "2026-09-27T23:59:59"}
  },
  "count": 1,
  "pagination": {"limit": 10, "offset": 0},
  "data": [
    {"id": 7, "name": "Лазерная депиляция", "description": "", "created_at": "2026-09-27T08:00:00Z"}
  ]
}
```

`filters` показывает, какие фильтры применились (`null` — не задан), `count` — сколько записей подходит всего, `data` — текущая страница.

Запись:

```json
{"id": 1, "name": "первая", "description": "", "created_at": "2026-09-26T23:33:25.360074+03:00"}
```

Примеры:

```sh
curl -i -X POST localhost:8080/records -d '{"name":"первая"}'
curl -i localhost:8080/records
curl -i localhost:8080/records/1
curl -i -X PUT localhost:8080/records/1 -d '{"name":"изменённая"}'
curl -i -X DELETE localhost:8080/records/1
```

## Деплой на Vercel

Vercel запускает приложение как serverless-функцию [api/index.go](api/index.go): все пути
перенаправляются в неё ([vercel.json](vercel.json)), дальше работает тот же роутер, что и в Docker.
Приложение собирается при первом запросе и переиспользуется, пока экземпляр функции жив.

1. [vercel.com/new](https://vercel.com/new) → импортировать репозиторий, настройки по умолчанию.
2. В проекте: **Storage → Create Database → Neon** (Postgres) → подключить к проекту.
   Vercel сам добавит `DATABASE_URL` и `DATABASE_URL_UNPOOLED`.
3. **Settings → Environment Variables**: `APP_TZ` = `Europe/Moscow`.
4. **Deployments → Redeploy**. Миграции применятся при первом запросе.

Дальше каждый push в `main` деплоится автоматически.

## Логи и health-check

Каждый запрос пишется в лог ([slog](https://pkg.go.dev/log/slog)):

```
level=INFO msg=request method=POST path=/records status=201 duration=944µs
```

Паника в хендлере не роняет соединение: клиент получает `500`, в лог — ошибка со стеком.
Docker проверяет контейнер через `/server healthcheck` (запрос к `/healthz`) — статус виден в `docker compose ps`.

## Миграции

SQL-миграции лежат в [migrations/](migrations/) и встроены в бинарник (`embed`).
При старте сервер сам применяет новые через [golang-migrate](https://github.com/golang-migrate/migrate);
применённые версии хранятся в таблице `schema_migrations`.

Чтобы изменить схему, добавь пару файлов со следующим номером:

```
migrations/000003_что_делает.up.sql     -- изменение
migrations/000003_что_делает.down.sql   -- откат
```

Уже применённые миграции не редактируй — только добавляй новые.

## Тесты

```sh
# тесты хендлеров — база не нужна
go test ./...

# плюс тесты хранилища на настоящей БД (нужен docker compose up -d)
TEST_DATABASE_URL="postgres://app:app@localhost:5433/go_education?sslmode=disable" go test -race ./...
```

Тесты хранилища работают во временной схеме и не трогают данные в базе.

На каждый push в `main` GitHub Actions запускает gofmt, `go vet`, все тесты с Postgres и сборку Docker-образа ([.github/workflows/ci.yml](.github/workflows/ci.yml)).

## Структура

```
cmd/server/          точка входа для Docker: HTTP-сервер, graceful shutdown
api/index.go         точка входа для Vercel (serverless-функция)
internal/app/         сборка приложения: миграции, БД, роуты (общая для сервера и Vercel)
internal/middleware/ логирование запросов, recover от паник
internal/health/     /healthz и проверка для Docker
internal/records/
  model.go           структура Record
  filter.go          фильтры списка и формат ответа
  storage.go         SQL-запросы к Postgres
  handler.go         HTTP-хендлеры и роуты
migrations/          SQL-миграции + код их применения
Dockerfile           сборка образа сервера (multi-stage, ~12 МБ)
docker-compose.yml   PostgreSQL (порт 5433) + сервер (порт 8080)
.github/workflows/   CI
```
