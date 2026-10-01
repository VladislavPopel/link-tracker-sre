package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	postgres_gorm "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/domain"
	ormrepo "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository/orm"
	sqlrepo "gitlab.education.tbank.ru/backend-academy-go-2025/homeworks/link-tracker/internal/infrastructure/repository/sql"
)

// startPostgres запускает PostgreSQL в Docker через Testcontainers
// и возвращает DSN для подключения.
func startPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	// Testcontainers сам скачает образ postgres:16 и запустит контейнер
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

	// Останавливаем контейнер после теста
	t.Cleanup(func() {
		if terminateErr := pgContainer.Terminate(ctx); terminateErr != nil {
			t.Logf("terminate postgres container: %v", terminateErr)
		}
	})

	dsn, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}
	return dsn
}

// applyMigrations применяет миграции к тестовой БД.
func applyMigrations(t *testing.T, sqlDB *sql.DB) {
	t.Helper()

	sourceDriver, err := iofs.New(os.DirFS("../../../migrations"), ".")
	if err != nil {
		t.Fatalf("create migration source: %v", err)
	}

	dbDriver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		t.Fatalf("create migration db driver: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		t.Fatalf("create migrator: %v", err)
	}

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("run migrations: %v", err)
	}
}

// ─── SQL тесты

func TestSQLRepo_AddAndGetLink(t *testing.T) {
	dsn := startPostgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	applyMigrations(t, db)

	repo := sqlrepo.NewLinkRepo(db)
	ctx := context.Background()

	if addChatErr := repo.AddChat(ctx, 1); addChatErr != nil {
		t.Fatalf("add chat: %v", addChatErr)
	}

	link, addLinkErr := repo.AddLink(ctx, &domain.Link{
		URL:    "https://github.com/user/repo",
		Tags:   []string{"work", "golang"},
		ChatID: 1,
	})
	if addLinkErr != nil {
		t.Fatalf("add link: %v", addLinkErr)
	}
	if link.ID == 0 {
		t.Error("expected non-zero link ID")
	}

	links, getLinksErr := repo.GetLinks(ctx, 1, domain.Page{Limit: 10, Offset: 0})
	if getLinksErr != nil {
		t.Fatalf("get links: %v", getLinksErr)
	}
	if len(links) != 1 {
		t.Fatalf("want 1 link, got %d", len(links))
	}
	if links[0].URL != "https://github.com/user/repo" {
		t.Errorf("wrong URL: %s", links[0].URL)
	}
	if len(links[0].Tags) != 2 {
		t.Errorf("want 2 tags, got %v", links[0].Tags)
	}
}

func TestSQLRepo_AddDuplicateLink(t *testing.T) {
	dsn := startPostgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	applyMigrations(t, db)

	repo := sqlrepo.NewLinkRepo(db)
	ctx := context.Background()

	if addChatErr := repo.AddChat(ctx, 1); addChatErr != nil {
		t.Fatalf("add chat: %v", addChatErr)
	}

	link := &domain.Link{URL: "https://github.com/user/repo", ChatID: 1}
	if _, addLinkErr := repo.AddLink(ctx, link); addLinkErr != nil {
		t.Fatalf("first add: %v", addLinkErr)
	}

	// Повторное добавление — должна быть ошибка ErrLinkExists
	_, addLinkErr := repo.AddLink(ctx, &domain.Link{URL: "https://github.com/user/repo", ChatID: 1})
	if !errors.Is(addLinkErr, domain.ErrLinkExists) {
		t.Errorf("want ErrLinkExists, got %v", addLinkErr)
	}
}

func TestSQLRepo_RemoveLink(t *testing.T) {
	dsn := startPostgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	applyMigrations(t, db)

	repo := sqlrepo.NewLinkRepo(db)
	ctx := context.Background()

	if addChatErr := repo.AddChat(ctx, 1); addChatErr != nil {
		t.Fatalf("add chat: %v", addChatErr)
	}
	if _, addLinkErr := repo.AddLink(ctx, &domain.Link{URL: "https://github.com/user/repo", ChatID: 1}); addLinkErr != nil {
		t.Fatalf("add link: %v", addLinkErr)
	}

	if _, removeLinkErr := repo.RemoveLink(ctx, 1, "https://github.com/user/repo"); removeLinkErr != nil {
		t.Fatalf("remove link: %v", removeLinkErr)
	}

	// Ссылка должна отсутствовать в БД
	links, getLinkErr := repo.GetLinks(ctx, 1, domain.Page{Limit: 10})
	if getLinkErr != nil {
		t.Fatalf("get links: %v", getLinkErr)
	}
	if len(links) != 0 {
		t.Errorf("expected empty list after remove, got %d links", len(links))
	}
}

func TestSQLRepo_RemoveChat(t *testing.T) {
	dsn := startPostgres(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	applyMigrations(t, db)

	repo := sqlrepo.NewLinkRepo(db)
	ctx := context.Background()

	if addChatErr := repo.AddChat(ctx, 1); addChatErr != nil {
		t.Fatalf("add chat: %v", addChatErr)
	}
	if removeChatErr := repo.RemoveChat(ctx, 999); !errors.Is(removeChatErr, domain.ErrChatNotFound) {
		t.Errorf("want ErrChatNotFound, got %v", removeChatErr)
	}
	if removeChatErr := repo.RemoveChat(ctx, 1); removeChatErr != nil {
		t.Fatalf("remove chat: %v", removeChatErr)
	}
}

// ─── ORM тесты

func newGORMDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(postgres_gorm.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return gdb
}

func TestORMRepo_AddAndGetLink(t *testing.T) {
	dsn := startPostgres(t)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()
	applyMigrations(t, sqlDB)

	repo := ormrepo.NewLinkRepo(newGORMDB(t, dsn))
	ctx := context.Background()

	if addChatErr := repo.AddChat(ctx, 1); addChatErr != nil {
		t.Fatalf("add chat: %v", addChatErr)
	}

	link, addLinkErr := repo.AddLink(ctx, &domain.Link{
		URL:    "https://github.com/user/repo",
		Tags:   []string{"work"},
		ChatID: 1,
	})
	if addLinkErr != nil {
		t.Fatalf("add link: %v", addLinkErr)
	}
	if link.ID == 0 {
		t.Error("expected non-zero link ID")
	}

	links, getLinksErr := repo.GetLinks(ctx, 1, domain.Page{Limit: 10})
	if getLinksErr != nil {
		t.Fatalf("get links: %v", getLinksErr)
	}
	if len(links) != 1 {
		t.Fatalf("want 1 link, got %d", len(links))
	}
}

func TestORMRepo_AddDuplicateLink(t *testing.T) {
	dsn := startPostgres(t)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()
	applyMigrations(t, sqlDB)

	repo := ormrepo.NewLinkRepo(newGORMDB(t, dsn))
	ctx := context.Background()

	if addChatErr := repo.AddChat(ctx, 1); addChatErr != nil {
		t.Fatalf("add chat: %v", addChatErr)
	}
	if _, addLinkErr := repo.AddLink(ctx, &domain.Link{URL: "https://github.com/user/repo", ChatID: 1}); addLinkErr != nil {
		t.Fatalf("first add: %v", addLinkErr)
	}

	_, addLinkErr := repo.AddLink(ctx, &domain.Link{URL: "https://github.com/user/repo", ChatID: 1})
	if !errors.Is(addLinkErr, domain.ErrLinkExists) {
		t.Errorf("want ErrLinkExists, got %v", addLinkErr)
	}
}

// ─── Тест переключения access-type

func TestAccessType_Switch(t *testing.T) {
	dsn := startPostgres(t)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()
	applyMigrations(t, sqlDB)
	ctx := context.Background()

	for i, accessType := range []string{"SQL", "ORM"} {
		t.Run(fmt.Sprintf("access-type=%s", accessType), func(t *testing.T) {
			var repo domain.LinkRepository
			switch accessType {
			case "SQL":
				repo = sqlrepo.NewLinkRepo(sqlDB)
			case "ORM":
				repo = ormrepo.NewLinkRepo(newGORMDB(t, dsn))
			}

			chatID := int64(100 + i)
			if addChatErr := repo.AddChat(ctx, chatID); addChatErr != nil {
				t.Fatalf("[%s] add chat: %v", accessType, addChatErr)
			}
			if _, addLinkErr := repo.AddLink(ctx, &domain.Link{
				URL:    "https://github.com/test/repo",
				ChatID: chatID,
			}); addLinkErr != nil {
				t.Fatalf("[%s] add link: %v", accessType, addLinkErr)
			}
			links, getLinksErr := repo.GetLinks(ctx, chatID, domain.Page{Limit: 10})
			if getLinksErr != nil {
				t.Fatalf("[%s] get links: %v", accessType, getLinksErr)
			}
			if len(links) != 1 {
				t.Errorf("[%s] want 1 link, got %d", accessType, len(links))
			}
		})
	}
}
