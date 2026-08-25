# qshqn

qshqn is **THE** telegram bot.

## core principles

### general
1. **brief, informal, non-capitalized text**

### code
1. **low ram usage**
2. **never return naked errors; wrap them up at every step**

### usage
1. **flexibility**
2. **brevity**
3. **usage simplicity**
4. **openness**

---

## Todo

### 1. `fiox`
- en/decryption
- refactor file load/save options ([No]SetCache and [No]ReadCache; i like its explicitness but also dont like it too)

### 2. `qsh`
- sometimes terminal breaks on exit
- cli commands (`qshqn [keyword]`)

### 3. `netx`
- nothing yet

### 4. `typex`
- expand MsgCache

### 5. `db`
- auto db & schema generation/validation
- auto struct read/write (currently has to read all structs all over again on startup; need to refactor)
- auto migrations
- auto backups

### 6. `ai`
- media scanner (images/gifs/stickers/voice messages/videos/circles/music/maybe chat tags and chat names)
- cache and reuse fileids
- media compression

### 7. `magahat`
- face/eye vision detector (preferably not just humans)
- extract eye coordinates, scale, angle
- procedural magahat drawing (match python implementation)

### 8. `command/quiz`
- implement

### 9. `locale`
- do something about countable nouns

### features/fixes
- custom command pipelines
- custom trigger word
- memories abt user
- find utility for duzhocoins
- fix dodep confirmation sometimes not registering correctly

---

## future plans
- support discord; maybe other platforms
- api
- do sth like a webapp/game
- support more languages
- add text games maybe. like crocodile etc.
