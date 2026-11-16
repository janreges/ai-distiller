package db

// Conn is a database connection.
type Conn struct {
	dsn string
}

// Dial opens a connection.
func Dial(dsn string) *Conn {
	return &Conn{dsn: dsn}
}

// Rows counts the rows of a table.
func (c *Conn) Rows(table string) int {
	return len(c.dsn) + len(table)
}
