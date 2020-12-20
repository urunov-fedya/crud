package customers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v4/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	//ErrNotFound возвращается, когда покупатель не найден.
	ErrNotFound = errors.New("item not found")

	//ErrInternal возвращается, когда произошла внутернняя ошибка.
	ErrInternal = errors.New("internal error")

	//ErrNoSuchUser возвращается, когда пользователь не найден
	ErrNoSuchUser = errors.New("no such user")

	//ErrInvalidPassword возвращается, когда пороль не верен
	ErrInvalidPassword = errors.New("invalid password")

	//ErrExpireToken возвращается, когда время ожидания токена истекает
	ErrExpireToken = errors.New("token expired")
)

//Service описывает сервис работы с покупателям.
type Service struct {
	pool *pgxpool.Pool
}

//NewService создаёт сервис.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

//Customer представляет информацию о покупателе.
type Customer struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Phone    string    `json:"phone"`
	Password string    `json:"password"`
	Active   bool      `json:"active"`
	Created  time.Time `json:"created"`
}

//All ....
func (s *Service) All(ctx context.Context) (cs []*Customer, err error) {

	sqlStatement := `SELECT * FROM customers`

	rows, err := s.pool.Query(ctx, sqlStatement)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {
		item := &Customer{}
		err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Phone,
			&item.Active,
			&item.Created,
		)
		if err != nil {
			log.Println(err)
		}

		cs = append(cs, item)
	}

	return cs, nil
}

//AllActive ....
func (s *Service) AllActive(ctx context.Context) (cs []*Customer, err error) {

	sqlStatement := `SELECT * FROM customers WHERE active`

	rows, err := s.pool.Query(ctx, sqlStatement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		item := &Customer{}
		err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Phone,
			&item.Active,
			&item.Created,
		)
		if err != nil {
			log.Println(err)
		}
		cs = append(cs, item)
	}

	return cs, nil
}

//ByID ...
func (s *Service) ByID(ctx context.Context, id int64) (*Customer, error) {
	item := &Customer{}

	sqlStatement := `SELECT id, name, phone, active, created FROM customers WHERE id = $1`
	err := s.pool.QueryRow(ctx, sqlStatement, id).Scan(
		&item.ID,
		&item.Name,
		&item.Phone,
		&item.Active,
		&item.Created,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		log.Print(err)
		return nil, ErrInternal
	}

	return item, nil
}

//ChangeActive ...
func (s *Service) ChangeActive(ctx context.Context, id int64, active bool) (*Customer, error) {
	item := &Customer{}

	sqlStatement := `UPDATE customers SET active = $2 where id = $1 RETURNING *`
	err := s.pool.QueryRow(ctx, sqlStatement, id, active).Scan(
		&item.ID,
		&item.Name,
		&item.Phone,
		&item.Active,
		&item.Created,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		log.Print(err)
		return nil, ErrInternal
	}
	return item, nil
}

//Delete ...
func (s *Service) Delete(ctx context.Context, id int64) (*Customer, error) {
	item := &Customer{}

	sqlStatement := `DELETE FROM customers WHERE id = $1 RETURNING *`
	err := s.pool.QueryRow(ctx, sqlStatement, id).Scan(
		&item.ID,
		&item.Name,
		&item.Phone,
		&item.Active,
		&item.Created,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		log.Print(err)
		return nil, ErrInternal
	}

	return item, nil
}

//Save ...
func (s *Service) Save(ctx context.Context, customer *Customer) (c *Customer, err error) {

	item := &Customer{}

	if customer.ID == 0 {
		sqlStatement := `INSERT INTO customers(name, phone, password) VALUES($1, $2, $3) RETURNING *`
		err = s.pool.QueryRow(ctx, sqlStatement, customer.Name, customer.Phone, customer.Password).Scan(
			&item.ID,
			&item.Name,
			&item.Phone,
			&item.Password,
			&item.Active,
			&item.Created,
		)
	} else {
		sqlStatement := `UPDATE customers SET name = $1, phone = $2, password = $3 WHERE id = $4 RETURNING *`
		err = s.pool.QueryRow(ctx, sqlStatement, customer.Name, customer.Phone, customer.Password, customer.ID).Scan(
			&item.ID,
			&item.Name,
			&item.Phone,
			&item.Password,
			&item.Active,
			&item.Created,
		)
	}

	if err != nil {
		log.Print(err)
		return nil, ErrInternal
	}

	return item, nil
}

// Auth - sign in
func (s *Service) Auth(login, password string) bool {
	query := `SELECT login, password FROM managers WHERE login=$1 and password=$2`

	err := s.pool.QueryRow(context.Background(), query, login, password).Scan(&login, &password)
	if err != nil {
		log.Print(err)
		return false
	}

	return true
}

//TokenForCustomer ....
func (s *Service) TokenForCustomer(ctx context.Context, phone, password string) (string, error) {

	var hash string
	var id int64

	err := s.pool.QueryRow(ctx, "SELECT id, password FROM customers WHERE phone = $1", phone).Scan(&id, &hash)

	if err == pgx.ErrNoRows {
		return "", ErrNoSuchUser
	}
	if err != nil {
		return "", ErrInternal
	}
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return "", ErrInvalidPassword
	}

	buffer := make([]byte, 256)
	n, err := rand.Read(buffer)
	if n != len(buffer) || err != nil {
		return "", ErrInternal
	}

	token := hex.EncodeToString(buffer)
	_, err = s.pool.Exec(ctx, "INSERT INTO customers_tokens(token, customer_id) VALUES($1, $2)", token, id)
	if err != nil {
		return "", ErrInternal
	}

	return token, nil
}

//AuthenticateCustomer ...
func (s *Service) AuthenticateCustomer(ctx context.Context, tkn string) (int64, error) {
	var id int64
	var expire time.Time

	err := s.pool.QueryRow(ctx, "SELECT customer_id, expire FROM customers_tokens WHERE token=$1", tkn).Scan(&id, &expire)
	if err == pgx.ErrNoRows {
		return 0, ErrNoSuchUser
	}
	if err != nil {
		return 0, ErrInternal
	}

	tNow := time.Now().Format("2006-01-02 15:04:05")
	tEnd := expire.Format("2006-01-02 15:04:05")

	if tNow > tEnd {
		return 0, ErrExpireToken
	}

	return id, nil
}
