# backend-store

Single Badger store to serve bite, transcription and any others. Is kinda bite-centric, so required values revolve around a Bite. Receives stores through [NATS](https://nats.io) while data is retrieved via a http api. Checks `backend-permissions` for permissions to access the store.

**Relies on being behind a traefik instance forwarding auth to backend-auth for authentication**

## Environment Variables

Supply environment variables by either exporting them or editing ```.env```.

| ENV | Description | Default |
| ---- | ----------- | ------- |
| DBPATH | Path to store badger files in. Please make sure it exists. | /tmp/badger |
| NATS | Host and port of nats | nats://localhost:4222 |
| LISTEN | Host and port to listen on | :80 |
| PERMISSIONS_HOST | URL of `backend-permissions` | http://permissions |

## Key format

Takes in three variables: ```type```, ```key``` and ```start```. Type is the type of data to be inserted, e.g. ```bite``` or ```transcription```. Key is the id of the conversation the bite was said in. Start is the Epoch timestamp of the start of the Bite.

## NATS

Refer to protobuf definitions in ```backend-protobuf```.

| Name | What you do | Accepted Protobuf |
| ---- | ----------- | ----------------- |
| store | Publish to | Store |

### store

Succeeds or fails quietly, just logging errors.

## API

| Contents |
| -------- |
| Scan Store |
| Get Store |

### Scan Store

```
GET /:type/:key/scan
```

Get a list of start times that one can use to query individual bites.

#### Params

| Name | Type | Description |
| ---- | ---- | ----------- |
| type | String | Type of store to query. I.e. `transcription` or `bite`. |
| key | String | Conversation ID of bite to query. |

#### Querystring

| Name | Type | Description |
| ---- | ---- | ----------- |
| from | Epoch timestamp | Time to start scanning from. |
| to | Epoch timestamp | Time to stop scanning at. |

#### Success (200 OK)

All numbers are Unix epoch timestamps. `starts` goes on for as long as it needs to.

```json
{
  "previous": 0,
  "starts: [0, 0, 0],
  "next": 0
}
```

#### Errors

| Code | Description |
| ---- | ----------- |
| 400 | From/to are not unix epoch/Error marshalling key from params. |
| 401 | `permissions` denied permission for user to access this store. |
| 500 | Error scanning badger store. |

---

### Get Store

```
GET /:type/:key/start/:start
```

Get the bite at a specific timestamp.

#### Params

| Name | Type | Description |
| ---- | ---- | ----------- |
| type | String | Type of store to query. I.e. `transcription` or `bite`. |
| key | String | Conversation ID of bite to query. |
| start | Epoch timestamp | Timestamp of the bite. |

#### Success (200 OK)

Raw data of the bite.

#### Errors

| Code | Description |
| ---- | ----------- |
| 400 | Error marshalling key from params/`start` was not a valid Epoch timestamp. |
| 401 | `permissions` denied permission for user to access this store. |
| 500 | Error retrieving bite from store. |
