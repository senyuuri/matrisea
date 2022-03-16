package vmm

import (
	"fmt"
	"log"
	"path"
	"time"

	"github.com/pkg/errors"
	bolt "go.etcd.io/bbolt"
)

var (
	DBFile          = "bolt.db"
	GlobalBucket    = []byte("global")
	ContainerBucket = []byte("container")
)

type KVStore struct {
	db *bolt.DB
}

type KeyValue struct {
	key   string
	value string
}

func NewKVStore(basePath string) *KVStore {
	dbPath := path.Join(basePath, DBFile)
	log.Printf("KVStore path %s\n", dbPath)
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatalf("Failed to create kvstore. Reason: %v", err)
	}
	return &KVStore{
		db: db,
	}
}

func (s *KVStore) PutContainterValue(containerName string, kvs []KeyValue) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		cbkt, err := tx.CreateBucketIfNotExists(ContainerBucket)
		if err != nil {
			return errors.Wrap(err, "fail to get container bucket")
		}
		// Get a nested bucket under ContainerBucket
		bkt, err := cbkt.CreateBucketIfNotExists([]byte(containerName))
		if err != nil {
			return errors.Wrap(err, "fail to get bucket "+containerName)
		}
		for _, kv := range kvs {
			err = bkt.Put([]byte(kv.key), []byte(kv.value))
			if err != nil {
				return errors.Wrap(err, "fail to put value")
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "fail to update db")
	}
	return nil
}

func (s *KVStore) GetContainerValue(containerName string, key string) (string, error) {
	var value []byte
	err := s.db.View(func(tx *bolt.Tx) error {
		cbkt := tx.Bucket(ContainerBucket)
		if cbkt == nil {
			return fmt.Errorf("container bucket not found")
		}
		bkt := cbkt.Bucket([]byte(containerName))
		if bkt == nil {
			return fmt.Errorf("bucket %s not found", containerName)
		}
		value = bkt.Get([]byte(key))
		if value == nil {
			return fmt.Errorf("key %s not found in %s", key, containerName)
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return string(value), nil
}

func (s *KVStore) GetContainerValueOrEmpty(containerName string, key string) string {
	var value string
	s.db.View(func(tx *bolt.Tx) error {
		cbkt := tx.Bucket(ContainerBucket)
		if cbkt != nil {
			bkt := cbkt.Bucket([]byte(containerName))
			if bkt != nil {
				v := bkt.Get([]byte(key))
				if v != nil {
					value = string(v)
				}
			}
		}
		return nil
	})
	return value
}

func (s *KVStore) RemoveContainerConfigs(containerName string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		cbkt := tx.Bucket(ContainerBucket)
		if cbkt != nil {
			return cbkt.DeleteBucket([]byte(containerName))
		}
		return nil
	})
}

func (s *KVStore) Close() error {
	return s.db.Close()
}
