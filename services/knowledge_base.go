package services

import (
    "context"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeDocument struct {
    ID        string
    Title     string
    Content   string
    Category  string
    Tags      []string
    CreatedAt time.Time
}

type KnowledgeBase struct {
    db *pgxpool.Pool
}

func NewKnowledgeBase(db *pgxpool.Pool) *KnowledgeBase {
    return &KnowledgeBase{db: db}
}

// LoadDirectory загружает все .md файлы из папки в БД
func (kb *KnowledgeBase) LoadDirectory(dirPath string) (int, error) {
    var loaded int
    err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            return nil
        }
        if !strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".txt") {
            return nil
        }

        if err := kb.loadMarkdownFile(path); err != nil {
            log.Printf("⚠️ Ошибка загрузки %s: %v", path, err)
            return nil
        }
        loaded++
        log.Printf("✅ Загружен: %s", path)
        return nil
    })

    return loaded, err
}

// loadMarkdownFile загружает один Markdown файл
func (kb *KnowledgeBase) loadMarkdownFile(filePath string) error {
    content, err := os.ReadFile(filePath)
    if err != nil {
        return err
    }

    text := string(content)

    // Парсим заголовок (первая строка с #)
    lines := strings.Split(text, "\n")
    title := strings.TrimPrefix(lines[0], "# ")
    if title == lines[0] {
        title = strings.TrimSuffix(filepath.Base(filePath), ".md")
        title = strings.TrimSuffix(title, ".txt")
    }

    // Определяем категорию из пути
    category := "general"
    if strings.Contains(filePath, "faq") {
        category = "faq"
    } else if strings.Contains(filePath, "integrations") {
        category = "integrations"
    } else if strings.Contains(filePath, "api") {
        category = "api"
    } else if strings.Contains(filePath, "platform") {
        category = "platform"
    } else if strings.Contains(filePath, "crm") {
        category = "crm"
    } else if strings.Contains(filePath, "finance") || strings.Contains(filePath, "fincore") {
        category = "finance"
    }

    _, err = kb.db.Exec(context.Background(), `
        INSERT INTO knowledge_documents (id, title, content, category, created_at, updated_at)
        VALUES ($1, $2, $3, $4, NOW(), NOW())
        ON CONFLICT (id) DO UPDATE SET
            title = EXCLUDED.title,
            content = EXCLUDED.content,
            category = EXCLUDED.category,
            updated_at = NOW()
    `, uuid.New().String(), title, text, category)

    return err
}

// SearchByKeyword поиск документов по ключевым словам
func (kb *KnowledgeBase) SearchByKeyword(query string, limit int) ([]KnowledgeDocument, error) {
    rows, err := kb.db.Query(context.Background(), `
        SELECT id, title, content, category, created_at
        FROM knowledge_documents
        WHERE content ILIKE $1 OR title ILIKE $1
        ORDER BY 
            CASE WHEN title ILIKE $1 THEN 1 ELSE 2 END,
            created_at DESC
        LIMIT $2
    `, "%"+query+"%", limit)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var docs []KnowledgeDocument
    for rows.Next() {
        var doc KnowledgeDocument
        err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &doc.Category, &doc.CreatedAt)
        if err != nil {
            continue
        }
        // Ограничиваем длину контента для контекста (1500 символов)
        if len(doc.Content) > 1500 {
            doc.Content = doc.Content[:1500] + "..."
        }
        docs = append(docs, doc)
    }
    return docs, nil
}

// SearchByCategory поиск документов по категории
func (kb *KnowledgeBase) SearchByCategory(category string, limit int) ([]KnowledgeDocument, error) {
    rows, err := kb.db.Query(context.Background(), `
        SELECT id, title, content, category, created_at
        FROM knowledge_documents
        WHERE category = $1
        ORDER BY created_at DESC
        LIMIT $2
    `, category, limit)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var docs []KnowledgeDocument
    for rows.Next() {
        var doc KnowledgeDocument
        err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &doc.Category, &doc.CreatedAt)
        if err != nil {
            continue
        }
        if len(doc.Content) > 1500 {
            doc.Content = doc.Content[:1500] + "..."
        }
        docs = append(docs, doc)
    }
    return docs, nil
}

// SaveQuery сохраняет запрос в историю
func (kb *KnowledgeBase) SaveQuery(userID, query, answer string, foundDocs []string) error {
    _, err := kb.db.Exec(context.Background(), `
        INSERT INTO knowledge_queries (user_id, query, found_documents, answer, created_at)
        VALUES ($1, $2, $3, $4, NOW())
    `, userID, query, foundDocs, answer)
    return err
}

// SetFeedback обратная связь: помог ответ или нет
func (kb *KnowledgeBase) SetFeedback(queryID string, helpful bool) error {
    _, err := kb.db.Exec(context.Background(), `
        UPDATE knowledge_queries SET helpful = $1 WHERE id = $2
    `, helpful, queryID)
    return err
}

// GetQueryHistory получить историю запросов пользователя
func (kb *KnowledgeBase) GetQueryHistory(userID string, limit int) ([]map[string]interface{}, error) {
    rows, err := kb.db.Query(context.Background(), `
        SELECT id, query, found_documents, answer, helpful, created_at
        FROM knowledge_queries
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `, userID, limit)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var history []map[string]interface{}
    for rows.Next() {
        var id, query, answer string
        var foundDocs interface{}
        var helpful *bool
        var createdAt time.Time

        err := rows.Scan(&id, &query, &foundDocs, &answer, &helpful, &createdAt)
        if err != nil {
            continue
        }

        history = append(history, map[string]interface{}{
            "id":              id,
            "query":           query,
            "found_documents": foundDocs,
            "answer":          answer,
            "helpful":         helpful,
            "created_at":      createdAt,
        })
    }
    return history, nil
}

// DeleteDocument удалить документ из базы знаний
func (kb *KnowledgeBase) DeleteDocument(docID string) error {
    _, err := kb.db.Exec(context.Background(), `
        DELETE FROM knowledge_documents WHERE id = $1
    `, docID)
    return err
}

// GetAllDocuments получить все документы
func (kb *KnowledgeBase) GetAllDocuments() ([]KnowledgeDocument, error) {
    rows, err := kb.db.Query(context.Background(), `
        SELECT id, title, content, category, created_at
        FROM knowledge_documents
        ORDER BY created_at DESC
    `)

    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var docs []KnowledgeDocument
    for rows.Next() {
        var doc KnowledgeDocument
        err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &doc.Category, &doc.CreatedAt)
        if err != nil {
            continue
        }
        docs = append(docs, doc)
    }
    return docs, nil
}