package main

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed ControlFlowTest.trash
var _sourceCode string

var _contentHash string

func init() {
	hash := sha256.Sum256([]byte(_sourceCode))
	_contentHash = hex.EncodeToString(hash[:])
}

var ErrUnknownSelector = errors.New("unknown selector")

type ControlFlowTest struct {
	Class     string   `json:"class"`
	CreatedAt string   `json:"created_at"`
	Vars      []string `json:"_vars"`
	Value     int      `json:"value"`
	Count     int      `json:"count"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: ControlFlowTest.native <instance_id> <selector> [args...]")
		fmt.Fprintln(os.Stderr, "       ControlFlowTest.native --source")
		fmt.Fprintln(os.Stderr, "       ControlFlowTest.native --hash")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "--source":
		fmt.Print(_sourceCode)
		return
	case "--hash":
		fmt.Println(_contentHash)
		return
	case "--info":
		fmt.Printf("Class: ControlFlowTest\nHash: %s\nSource length: %d bytes\n", _contentHash, len(_sourceCode))
		return
	}

	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: ControlFlowTest.native <instance_id> <selector> [args...]")
		os.Exit(1)
	}

	receiver := os.Args[1]
	selector := os.Args[2]
	args := os.Args[3:]

	if receiver == "ControlFlowTest" || receiver == "ControlFlowTest" {
		result, err := dispatchClass(selector, args)
		if err != nil {
			if errors.Is(err, ErrUnknownSelector) {
				os.Exit(200)
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if result != "" {
			fmt.Println(result)
		}
		return
	}

	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	instance, err := loadInstance(db, receiver)
	if err != nil {
		os.Exit(200)
	}

	result, err := dispatch(instance, selector, args)
	if err != nil {
		if errors.Is(err, ErrUnknownSelector) {
			os.Exit(200)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := saveInstance(db, receiver, instance); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving instance: %v\n", err)
		os.Exit(1)
	}

	if result != "" {
		fmt.Println(result)
	}
}

func openDB() (*sql.DB, error) {
	dbPath := os.Getenv("SQLITE_JSON_DB")
	if dbPath == "" {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, ".trashtalk", "instances.db")
	}
	return sql.Open("sqlite3", dbPath)
}

func loadInstance(db *sql.DB, id string) (*ControlFlowTest, error) {
	var data string
	err := db.QueryRow("SELECT data FROM instances WHERE id = ?", id).Scan(&data)
	if err != nil {
		return nil, err
	}
	var instance ControlFlowTest
	if err := json.Unmarshal([]byte(data), &instance); err != nil {
		return nil, err
	}
	return &instance, nil
}

func saveInstance(db *sql.DB, id string, instance *ControlFlowTest) error {
	data, err := json.Marshal(instance)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT OR REPLACE INTO instances (id, data) VALUES (?, json(?))", id, string(data))
	return err
}

func sendMessage(receiver interface{}, selector string, args ...interface{}) (string, error) {
	receiverStr := fmt.Sprintf("%v", receiver)
	cmdArgs := []string{receiverStr, selector}
	for _, arg := range args {
		cmdArgs = append(cmdArgs, fmt.Sprintf("%v", arg))
	}
	home, _ := os.UserHomeDir()
	dispatchScript := filepath.Join(home, ".trashtalk", "bin", "trash-send")
	cmd := exec.Command(dispatchScript, cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func dispatch(c *ControlFlowTest, selector string, args []string) (string, error) {
	switch selector {
	case "testIfTrue":
		return c.TestIfTrue(), nil
	case "testIfElse":
		return c.TestIfElse(), nil
	case "testComparison":
		return c.TestComparison(), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownSelector, selector)
	}
}

func dispatchClass(selector string, args []string) (string, error) {
	switch selector {
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownSelector, selector)
	}
}

func (c *ControlFlowTest) TestIfTrue() string {
	if c.Value > 5 {
		c.Count = 1
	}
	return strconv.Itoa(c.Count)
}

func (c *ControlFlowTest) TestIfElse() string {
	var result int
	if c.Value >= 10 {
		result = 100
	} else {
		result = 0
	}
	return strconv.Itoa(result)
}

func (c *ControlFlowTest) TestComparison() string {
	var result int
	if c.Value == 0 {
		result = 1
	} else {
		result = 2
	}
	return strconv.Itoa(result)
}
