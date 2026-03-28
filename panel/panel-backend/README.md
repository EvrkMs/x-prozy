# X-Prozy Panel Backend

## Проект улучшен: GORM ORM + правильная структура

### 📁 Структура проекта

```
internal/
├── auth/                # Сервис аутентификации
│   └── service.go       # Основная логика аутентификации
├── config/              # Конфигурация приложения
│   └── config.go        # Загрузка конфига из переменных окружения
├── database/            # База данных (GORM)
│   └── db.go            # Подключение и миграции
├── handlers/            # HTTP handlers
│   └── auth_handler.go  # Handlers для аутентификации
├── middleware/          # HTTP middleware
│   └── auth.go          # Middleware для проверки сессий
├── models/              # Модели данных GORM
│   ├── user.go          # Модель User
│   ├── session.go       # Модель Session
│   ├── audit_log.go     # Модель AuditLog
│   └── api_response.go  # API response структуры
├── repository/          # Слой доступа к данным (репозитории)
│   ├── interfaces.go    # Интерфейсы репозиториев
│   ├── user_repo.go     # User репозиторий
│   ├── session_repo.go  # Session репозиторий
│   └── audit_log_repo.go # AuditLog репозиторий
└── web/                 # Веб приложение
    └── app.go           # Инициализация и маршруты
cmd/
└── panel/
    └── main.go          # Точка входа
pkg/
└── errors/              # Пакет с текстовыми ошибками
    └── errors.go        # Определение ошибок
```

### 🔒 Безопасность SQL запросов

Все SQL запросы теперь выполняются через **GORM ORM**:
- ✅ Защита от SQL injection (параметризованные запросы)
- ✅ Автоматические миграции
- ✅ Связи между таблицами
- ✅ Типобезопасность

### 🏗️ Архитектура

**Четыёхслойная архитектура:**

1. **HTTP Handlers** (`handlers/`) - обработка HTTP запросов
2. **Auth Service** (`auth/`) - бизнес-логика аутентификации
3. **Repositories** (`repository/`) - доступ к данным
4. **Database** (`database/`) - соединение с БД и миграции

### 🗂️ Модели данных

#### User
- `ID` - уникальный идентификатор
- `Username` - имя пользователя (уникальное)
- `Email` - email (уникальный)
- `PasswordHash` - хеш пароля (bcrypt)
- `IsAdmin` - флаг администратора
- `IsActive` - активен ли пользователь
- `LastLogin` - время последнего входа
- `CreatedAt`, `UpdatedAt`, `DeletedAt` - timestamps

#### Session
- `ID` - уникальный идентификатор
- `Token` - сессионный токен
- `UserID` - ID пользователя (внешний ключ)
- `ExpiresAt` - время истечения сессии
- Автоматическое удаление при удалении пользователя

#### AuditLog
- `ID` - уникальный идентификатор
- `UserID` - ID пользователя (может быть NULL для анонимных действий)
- `Action` - тип действия (login, logout, create, update, delete, access)
- `Resource` - на какой ресурс действие
- `Details` - JSON с дополнительными деталями
- `IPAddress` - IP адрес

### 🔑 Интерфейсы репозиториев

Репозитории реализуют интерфейсы для легкого мокирования при тестировании:

```go
type UserRepository interface {
    Create(ctx context.Context, user *models.User) error
    GetByID(ctx context.Context, id int64) (*models.User, error)
    GetByUsername(ctx context.Context, username string) (*models.User, error)
    GetByEmail(ctx context.Context, email string) (*models.User, error)
    Update(ctx context.Context, user *models.User) error
    Delete(ctx context.Context, id int64) error
    ListActive(ctx context.Context) ([]models.User, error)
}
```

### 📝 Пример использования

```go
// Инициализация
db, _ := database.New("./panel.db")
userRepo := repository.NewUserRepo(db.DB)
sessionRepo := repository.NewSessionRepo(db.DB)
auditRepo := repository.NewAuditLogRepo(db.DB)

authService := auth.NewService(userRepo, sessionRepo, auditRepo, auth.ServiceConfig{
    SessionDuration: 7 * 24 * time.Hour,
})

// Аутентификация
token, user, err := authService.Authenticate(ctx, "admin", "admin", "127.0.0.1")

// Валидация сессии
user, err := authService.ValidateSession(ctx, token)

// Смена пароля
err := authService.ChangePassword(ctx, userID, oldPass, newPass, "127.0.0.1")

// Logout
err := authService.Logout(ctx, token, userID, "127.0.0.1")
```

### 🚀 Запуск

```bash
# Установка зависимостей
go mod download

# Компиляция
go build -o panel ./cmd/panel

# Запуск
./panel

# С переменными окружения
DB_PATH=./data/panel.db PANEL_ADDR=0.0.0.0 PANEL_PORT=8080 ./panel
```

### 📋 Переменные окружения

```
DB_PATH=./data/panel.db                    # Путь к БД SQLite
SESSION_DURATION=168h                       # Длительность сессии
SESSION_SAME_SITE=Lax                      # SameSite для cookies
SESSION_SECURE=false                       # Использовать Secure флаг для cookies
PANEL_ADDR=0.0.0.0                         # Адрес слушания
PANEL_PORT=8080                            # Порт слушания
```

### ✨ Преимущества новой структуры

- ✅ **Разделение ответственности** - каждый слой отвечает за одно
- ✅ **GORM для безопасности** - никаких SQL injection уязвимостей
- ✅ **Тестируемость** - все репозитории через интерфейсы
- ✅ **Масштабируемость** - легко добавить новые модели и репозитории
- ✅ **Аудит** - все действия логируются в AuditLog
- ✅ **Типобезопасность** - правильная типизация через GORM модели
