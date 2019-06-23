package main

import (
  "encoding/json"
  "log"
  "net/http"
  "os"

  . "store/backend-protobuf/go"

  "github.com/joho/godotenv"
  "github.com/dgraph-io/badger"
  "github.com/nats-io/go-nats"
  "github.com/golang/protobuf/proto"
)

var dbPath string
var natsHost string

var db *badger.DB
var nc *nats.Conn

func main() {
  // Load .env
  err := godotenv.Load()
  if err != nil {
    log.Fatal("Error loading .env file")
  }
  dbPath = os.Getenv("DBPATH")
  natsHost = os.Getenv("NATS")

  // Open badger
	log.Printf("starting badger at %s", dbPath)
	opts := badger.DefaultOptions
	opts.Dir = dbPath
	opts.ValueDir = dbPath
	db, err = badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

  // NATS client
  nc, err = nats.Connect(natsHost)
  if err != nil {
		log.Fatal(err)
	}
  nc.Subscribe("new_store", NewStore)

  nc.Subscribe("request_store", RequestStore)
  nc.Subscribe("scan_store", ScanStore)
  defer nc.Close()

  select { } // Wait forever
}

func NewStore(m *nats.Msg) {
  storeRequest := Store{}
  if err := proto.Unmarshal(m.Data, &storeRequest); err != nil {
    log.Println(err) // Fail quietly since protobuf data is needed torespond
    return
  }

  key, err := MarshalKey(storeRequest.Type, storeRequest.Bite.Key, storeRequest.Bite.Start)
  if err != nil {
    log.Println(err)
    errRes := Response {
      Code: 400,
      Message: []byte(http.StatusText(http.StatusBadRequest)),
      Client: storeRequest.Bite.Client,
    }
    errResBytes, errResErr := proto.Marshal(&errRes)
    if errResErr == nil {
      nc.Publish("res", errResBytes)
    }
    return
  }

  err = db.Update(func(txn *badger.Txn) error {
		// TODO: prevent overwriting existing
		err := txn.Set(key, storeRequest.Bite.Data)
		return err
	})

  if err != nil {
    log.Println(err)
    errRes := Response {
      Code: 500,
      Message: []byte(http.StatusText(http.StatusInternalServerError)),
      Client: storeRequest.Bite.Client,
    }
    errResBytes, errResErr := proto.Marshal(&errRes)
    if errResErr == nil {
      nc.Publish("res", errResBytes)
    }
    return
  } else {
    res := Response {
      Code: 200,
      Message: []byte(key),
      Client: storeRequest.Bite.Client,
    }
    resBytes, resErr := proto.Marshal(&res)
    if resErr == nil {
      nc.Publish("res", resBytes)
    }
  }
}

func RequestStore(m *nats.Msg) {
  req := DataRequest{}
  if err := proto.Unmarshal(m.Data, &req); err != nil {
    log.Println(err)
    return
  }

  key, err := MarshalKey(req.Type, req.Key, req.Start)

  err = db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
    value, err := item.Value()
    if err != nil {
      return err
    }

    res := Response {
      Code: 200,
      Message: value,
    }
    resBytes, err := proto.Marshal(&res)

    if err != nil {
      return err
    }

    nc.Publish(m.Reply, resBytes)
    return nil
	})

	if err != nil {
    res := ReplyError(err.Error(), 400)
    nc.Publish(m.Reply, res)
	}
}

type BitesList struct {
	Previous uint64   `json:"previous"` // One bite before starts. Hint for how many steps the client can skip
	Starts   []uint64 `json:"starts"`
	Next     uint64   `json:"next"` // One bite after starts. Hint for how many steps the client can skip
}

func ScanStore(m *nats.Msg) {
  req := ScanRequest {}
  if err := proto.Unmarshal(m.Data, &req); err != nil {
    log.Println(err)
    return
  }

  prefix, err := MarshalKeyPrefix(req.Type, req.Key)

  if err != nil {
    res := ReplyError(http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    nc.Publish(m.Reply, res)
    return
  }

  fromKey, err := MarshalKey(req.Type, req.Key, req.From)
	if err != nil {
    res := ReplyError(http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    nc.Publish(m.Reply, res)
		return
	}

  bitesList := BitesList {}

  err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		// Fetch previous key
		it.Seek(fromKey)
		if it.ValidForPrefix(fromKey) {
			// Lazy check to compare key == seeked key
			it.Next()
		}
		if !it.ValidForPrefix(prefix) {
			return nil
		}
		item := it.Item()
		key := item.Key()

		_, _, start, err := ExtractKey(key)
		if err != nil {
			return nil
		}
		bitesList.Previous = start

		return nil
	})
	if err != nil {
    res := ReplyError(err.Error(), http.StatusBadRequest)
    nc.Publish(m.Reply, res)
		return
	}

  err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(fromKey); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			_, _, start, err := ExtractKey(key)
			if err != nil {
				continue
			}
			if start > req.To {
				// A key was found that is greater than to
				// Save that as next
				bitesList.Next = start
				break
			}

			bitesList.Starts = append(bitesList.Starts, start)
		}

		return nil
	})
	if err != nil {
    res := ReplyError(err.Error(), http.StatusBadRequest)
    nc.Publish(m.Reply, res)
		return
	}

	jsonString, err := json.Marshal(&bitesList)
  res := Response {
    Code: 200,
    Message: []byte(jsonString),
  }
  resBytes, _ := proto.Marshal(&res)
  nc.Publish(m.Reply, resBytes)
}

func ReplyError(msg string, code uint32) []byte {
  res := Response {
    Code: code,
    Message: []byte(msg),
  }
  resBytes, _ := proto.Marshal(&res)

  return resBytes
}
