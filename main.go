package main

import (
  "context"
  "encoding/json"
  "log"
  "net/http"
  "os"
  "strconv"

  . "store/backend-protobuf/go"

  "github.com/joho/godotenv"
  "github.com/dgraph-io/badger"
  "github.com/nats-io/go-nats"
  "github.com/golang/protobuf/proto"
  "github.com/julienschmidt/httprouter"
)

var listen string
var dbPath string
var natsHost string
var permissionsHost string

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
  listen = os.Getenv("LISTEN")
  permissionsHost = os.Getenv("PERMISSIONS_HOST")

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
  nc.Subscribe("store", NewStore)
  defer nc.Close()

  // Routes
  router := httprouter.New()
  router.GET("/:type/:key/scan", AuthMiddleware(PermissionMiddleware(ScanStore)))
  router.GET("/:type/:key/start/:start", AuthMiddleware(PermissionMiddleware(GetStore)))

  // Start server
  log.Printf("starting server on %s", listen)
  log.Fatal(http.ListenAndServe(listen, router))
}

type RawClient struct {
  UserId   string `json:"userid"`
  ClientId string `json:"clientid"`
}
func AuthMiddleware(next httprouter.Handle) httprouter.Handle {
  return func (w http.ResponseWriter, r *http.Request, p httprouter.Params) {
    ua := r.Header.Get("X-User-Claim")
    if ua == "" {
      http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
      return
    }

    var client RawClient
    err := json.Unmarshal([]byte(ua), &client)
    if err != nil {
      http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
      return
    }

    context := context.WithValue(r.Context(), "user", client.UserId)
    next(w, r.WithContext(context), p)
  }
}

func PermissionMiddleware(next httprouter.Handle) httprouter.Handle {
  return func (w http.ResponseWriter, r *http.Request, p httprouter.Params) {
    userID := r.Context().Value("user").(string)
    conversationID := p.ByName("key")

    response, err := http.Get(permissionsHost + "/user/" + userID + "/conversation/" + conversationID)
    if err != nil {
      http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
      return
    }
    response.Body.Close()
    next(w, r, p)
  }
}

func NewStore(m *nats.Msg) {
  storeRequest := Store{}
  if err := proto.Unmarshal(m.Data, &storeRequest); err != nil {
    log.Println(err) // Just log errors
    return
  }

  key, err := MarshalKey(storeRequest.Type, storeRequest.Bite.Key, storeRequest.Bite.Start)
  if err != nil {
    log.Println(err)
    return
  }

  err = db.Update(func(txn *badger.Txn) error {
		// TODO: prevent overwriting existing
		err := txn.Set(key, storeRequest.Bite.Data)
		return err
	})
  if err != nil {
    log.Println(err)
    return
  }
}

func ParseStartString(start string) (uint64, error) {
  return strconv.ParseUint(start, 10, 64)
}

func GetStore(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
  // Get params
  storeType := p.ByName("type")
  key := p.ByName("key")

  start, err := ParseStartString(p.ByName("start"))
  if err != nil {
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return
  }

  storeKey, err := MarshalKey(storeType, key, start)
  if err != nil {
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return
  }

  err = db.View(func(txn *badger.Txn) error {
    item, err := txn.Get(storeKey)
    if err != nil {
      return err
    }

    value, err := item.Value()
    if err != nil {
      return err
    }

    w.Write(value)
    return nil
  })

  if err != nil {
    http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
    return
  }
}

type BitesList struct {
	Previous uint64   `json:"previous"` // One bite before starts. Hint for how many steps the client can skip
	Starts   []uint64 `json:"starts"`
	Next     uint64   `json:"next"` // One bite after starts. Hint for how many steps the client can skip
}

func ScanStore(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
  // Get params
  storeType := p.ByName("type")
  key := p.ByName("key")

  // Get querystring values
  from, err := ParseStartString(r.FormValue("from"))
  if err != nil {
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return
  }

  to, err := ParseStartString(r.FormValue("to"))
  if err != nil {
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return
  }

  prefix, err := MarshalKeyPrefix(storeType, key)
  if err != nil {
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return
  }

  fromKey, err := MarshalKey(storeType, key, from)
  if err != nil {
    http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
    return
  }

  bitesList := BitesList{}

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
    http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
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
			if start > to {
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
    http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
    return
  }

  // Respond
  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(bitesList)
}

