package db

import (
	"context"
	"crypto/rand"
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/textileio/go-threads/common"
	lstore "github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/util"
)

var (
	jsonSchema = `{
		"$schema": "http://json-schema.org/draft-04/schema#",
		"$ref": "#/definitions/person",
		"definitions": {
			"person": {
				"required": [
					"_id",
					"name",
					"age"
				],
				"properties": {
					"_id": {
						"type": "string"
					},
					"name": {
						"type": "string"
					},
					"age": {
						"type": "integer"
					}
				},
				"additionalProperties": false,
				"type": "object"
			}
		}
	}`
)

func TestManager_GetToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	man, clean := createTestManager(t)
	defer clean()

	sk, _, err := crypto.GenerateEd25519Key(rand.Reader)
	checkErr(t, err)
	tok, err := man.GetToken(ctx, thread.NewLibp2pIdentity(sk))
	checkErr(t, err)
	if tok == "" {
		t.Fatal("bad token")
	}
}

func TestManager_NewDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("test one new db", func(t *testing.T) {
		t.Parallel()
		man, clean := createTestManager(t)
		defer clean()
		_, err := man.NewDB(ctx, thread.NewIDV1(thread.Raw, 32))
		checkErr(t, err)
	})
	t.Run("test multiple new dbs", func(t *testing.T) {
		t.Parallel()
		man, clean := createTestManager(t)
		defer clean()
		_, err := man.NewDB(ctx, thread.NewIDV1(thread.Raw, 32))
		checkErr(t, err)
		// NewDB with token
		sk, _, err := crypto.GenerateEd25519Key(rand.Reader)
		checkErr(t, err)
		tok, err := man.GetToken(ctx, thread.NewLibp2pIdentity(sk))
		checkErr(t, err)
		_, err = man.NewDB(ctx, thread.NewIDV1(thread.Raw, 32), WithNewManagedDBToken(tok))
		checkErr(t, err)
	})
}

func TestManager_GetDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir, err := ioutil.TempDir("", "")
	checkErr(t, err)
	n, err := common.DefaultNetwork(dir, common.WithNetDebug(true), common.WithNetHostAddr(util.FreeLocalAddr()))
	checkErr(t, err)
	man, err := NewManager(n, WithNewDBRepoPath(dir), WithNewDBDebug(true))
	checkErr(t, err)
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	id := thread.NewIDV1(thread.Raw, 32)
	_, err = man.NewDB(ctx, id)
	checkErr(t, err)
	db, err := man.GetDB(ctx, id)
	checkErr(t, err)
	if db == nil {
		t.Fatal("db not found")
	}

	// Register a schema and create an instance
	collection, err := db.NewCollection(CollectionConfig{Name: "Person", Schema: util.SchemaFromSchemaString(jsonSchema)})
	checkErr(t, err)
	person1 := []byte(`{"_id": "", "name": "foo", "age": 21}`)
	_, err = collection.Create(person1)
	checkErr(t, err)

	time.Sleep(time.Second)

	// Close it down, restart next
	err = man.Close()
	checkErr(t, err)
	err = n.Close()
	checkErr(t, err)

	t.Run("test get db after restart", func(t *testing.T) {
		n, err := common.DefaultNetwork(dir, common.WithNetDebug(true), common.WithNetHostAddr(util.FreeLocalAddr()))
		checkErr(t, err)
		man, err := NewManager(n, WithNewDBRepoPath(dir), WithNewDBDebug(true))
		checkErr(t, err)

		db, err := man.GetDB(ctx, id)
		checkErr(t, err)
		if db == nil {
			t.Fatal("db was not hydrated")
		}

		// Add another instance, this time there should be no need to register the schema
		collection := db.GetCollection("Person")
		if collection == nil {
			t.Fatal("collection was not hydrated")
		}
		person2 := []byte(`{"_id": "", "name": "bar", "age": 21}`)
		person3 := []byte(`{"_id": "", "name": "baz", "age": 21}`)
		_, err = collection.CreateMany([][]byte{person2, person3})
		checkErr(t, err)

		time.Sleep(time.Second)

		err = man.Close()
		checkErr(t, err)
		err = n.Close()
		checkErr(t, err)
	})
}

func TestManager_DeleteDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	man, clean := createTestManager(t)
	defer clean()

	id := thread.NewIDV1(thread.Raw, 32)
	db, err := man.NewDB(ctx, id)
	checkErr(t, err)

	// Register a schema and create an instance
	collection, err := db.NewCollection(CollectionConfig{Name: "Person", Schema: util.SchemaFromSchemaString(jsonSchema)})
	checkErr(t, err)
	person1 := []byte(`{"_id": "", "name": "foo", "age": 21}`)
	_, err = collection.Create(person1)
	checkErr(t, err)

	time.Sleep(time.Second)

	err = man.DeleteDB(ctx, id)
	checkErr(t, err)

	_, err = man.GetDB(ctx, id)
	if !errors.Is(err, lstore.ErrThreadNotFound) {
		t.Fatal("db was not deleted")
	}
}

func createTestManager(t *testing.T) (*Manager, func()) {
	dir, err := ioutil.TempDir("", "")
	checkErr(t, err)
	n, err := common.DefaultNetwork(dir, common.WithNetDebug(true), common.WithNetHostAddr(util.FreeLocalAddr()))
	checkErr(t, err)
	m, err := NewManager(n, WithNewDBRepoPath(dir), WithNewDBDebug(true))
	checkErr(t, err)
	return m, func() {
		if err := n.Close(); err != nil {
			panic(err)
		}
		if err := m.Close(); err != nil {
			panic(err)
		}
		_ = os.RemoveAll(dir)
	}
}