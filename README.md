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

Переменные окружения сервера: `DATABASE_URL` (обязательна), `PORT` (по умолчанию `8080`).
По Ctrl+C / `docker stop` сервер дожидается текущих запросов и завершается корректно.

## API

| Метод    | Путь            | Тело               | Ответ                        |
|----------|-----------------|--------------------|------------------------------|
| `GET`    | `/records`      | —                  | `200` список записей         |
| `POST`   | `/records`      | `{"name":"..."}`   | `201` созданная запись       |
| `GET`    | `/records/{id}` | —                  | `200` запись                 |
| `PUT`    | `/records/{id}` | `{"name":"..."}`   | `200` обновлённая запись     |
| `DELETE` | `/records/{id}` | —                  | `204` без тела               |

Ошибки: `400` — неверный id, JSON или пустое имя; `404` — записи нет; `500` — ошибка сервера.

Запись:

```json
{"id": 1, "name": "первая", "created_at": "2026-09-26T23:33:25.360074+03:00"}
```

Примеры:

```sh
curl -i -X POST localhost:8080/records -d '{"name":"первая"}'
curl -i localhost:8080/records
curl -i localhost:8080/records/1
curl -i -X PUT localhost:8080/records/1 -d '{"name":"изменённая"}'
curl -i -X DELETE localhost:8080/records/1
```

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
cmd/server/          точка входа: подключение к БД, HTTP-сервер, graceful shutdown
internal/records/
  model.go           структура Record
  storage.go         SQL-запросы к Postgres
  handler.go         HTTP-хендлеры и роуты
migrations/          SQL-схема базы
Dockerfile           сборка образа сервера (multi-stage, ~12 МБ)
docker-compose.yml   PostgreSQL (порт 5433) + сервер (порт 8080)
.github/workflows/   CI
```
