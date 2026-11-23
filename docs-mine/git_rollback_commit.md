Да. Тебе нужно сначала поставить метку-ветку на текущий HEAD, чтобы два последних коммита не потерялись, а затем текущую ветку откатить на b8437e2....

Допустим, текущая ветка — v6-mine, а ветку с двумя последними коммитами назовём mail-server.

# 1. Создаём новую ветку на текущем состоянии,
# но НЕ переключаемся на неё
git branch mail-server

# 2. Проверяем, что ветка создана
git branch -v

# 3. Откатываем текущую ветку v6-mine на нужный коммит
git reset --hard b8437e2664c583bb75e9ae86470a4d214c8911ae

В итоге история будет примерно такой:

                     mail-server
                         ↓
b8437e2 ── commit 1 ── commit 2
   ↑
 v6-mine

То есть:

v6-mine → вернётся ровно к b8437e2664c583bb75e9ae86470a4d214c8911ae;
mail-server → сохранит оба последних коммита;
ты останешься на v6-mine;
переключения на mail-server не будет.
Если v6-mine уже запушена на GitHub

После reset локальная и удалённая v6-mine разойдутся. Если ты действительно хочешь откатить и GitHub-ветку, затем:

git push --force-with-lease origin v6-mine

А если хочешь также сохранить mail-server на GitHub:

git push -u origin mail-server

Это тоже не переключит тебя на mail-server.

Я бы в твоём случае сделал именно в таком порядке:

git branch mail-server
git reset --hard b8437e2664c583bb75e9ae86470a4d214c8911ae
git push --force-with-lease origin v6-mine
git push -u origin mail-server

Только перед reset --hard проверь git status: незакоммиченные изменения он удалит.