package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/go-sql-driver/mysql"
)

// DB - псевдоним для sql.DB, чтобы не импортировать пакет sql в других местах
type DB = sql.DB

// SQLResponse для результатов всех функций этой библиотеки
type SQLResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	StatusCode   int    `json:"statuscode"`
	Duration     int64  `json:"duration,omitempty"`
	RowsAffected int64  `json:"rows_affected,omitempty"`
}

// mainAnswer вспомогательная функция для сокращения строк возврата универсального результата
func mainAnswer(success bool, message string, statusCode int, duration int64, rowsAffected int64) (answ SQLResponse) {
	answ.Success = success
	answ.Message = message
	answ.StatusCode = statusCode
	answ.Duration = duration
	answ.RowsAffected = rowsAffected
	return answ
}

// получение соединения с базой данных для всех страждущих процессов
func GetDBConnection(conn **sql.DB, serverName string, port int, dbName string, dbUser string, dbPass string) error {
	connString := fmt.Sprintf("server=%s;port=%d;database=%s;user id=%s;password=%s;encrypt=disable",
		serverName, port, dbName, dbUser, dbPass)

	var err error
	//обновляем коннект, если он потерялся
	if *conn == nil {
		*conn, err = sql.Open("sqlserver", connString)
		if err != nil {
			return err
		}
	}

	// Проверяем соединение с базой данных
	err = (*conn).Ping()
	if err != nil {
		return err
	}

	(*conn).SetConnMaxLifetime(time.Minute * 3)
	(*conn).SetMaxOpenConns(100)
	(*conn).SetMaxIdleConns(100)

	return nil

}

// универсальная функция sql select запроса
func AnySelect[T any](db *sql.DB, query string, timeout time.Duration, args ...any) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []T

	for rows.Next() {
		var item T

		if err := scanStruct(rows, &item); err != nil {
			return nil, err
		}

		result = append(result, item)
	}

	return result, rows.Err()
}

func scanStruct(rows *sql.Rows, dest any) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return errors.New("dest must be pointer")
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return errors.New("dest must point to struct")
	}

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	t := v.Type()

	fields := make(map[string]int)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		name := f.Tag.Get("db")
		if name == "" {
			name = f.Name
		}

		fields[strings.ToLower(name)] = i
	}

	values := make([]any, len(columns))

	for i, col := range columns {
		if idx, ok := fields[strings.ToLower(col)]; ok {
			values[i] = v.Field(idx).Addr().Interface()
		} else {
			// если столбца нет в структуре
			var dummy any
			values[i] = &dummy
		}
	}

	return rows.Scan(values...)
}

// AnyExecQuery принимает вариативный список параметров (args...)
func AnyExecQuery(conn *sql.DB, query string, timeout time.Duration, args ...interface{}) (answ SQLResponse) {
	// Контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	result, err := conn.ExecContext(ctx, query, args...)

	if err != nil {
		// Проверка ошибок подключения или выполнения
		return mainAnswer(false, err.Error(), 500, 0, 0)
	}

	// Если нам нужно получить, сколько строк затронуто (rowCount)
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return mainAnswer(false, "Could not check rows affected", 500, 0, 0)
	}

	// Успешный возврат
	answ.Success = true
	answ.Message = fmt.Sprintf("Rows affected: %d", rowsAffected)
	answ.StatusCode = 200
	answ.RowsAffected = rowsAffected
	return answ
}
