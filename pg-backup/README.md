# Скрипт бэкапа и отправки postgres данных из docker-compose в Backblaze B2

## Включение крона для регулярных бэкапов с использованием выгрузки в Backblaze B2

Делаем скрипт исполняемым:
```
chmod +x ./script.sh
```

```bash
sudo crontab -e
```

В конец файла добавляем: 
```bash
30 3 * * * /root/Repos/family-app-go/pg-backup/script.sh >> /var/log/pg_backup.log 2>&1
# Для теста можно добавить 
* * * * * /root/Repos/family-app-go/pg-backup/script.sh >> /var/log/pg_backup.log 2>&1
```

Проверяем что добавилось задание крона:
```bash
sudo crontab -l
```


## Восстановление базы:
```bash
createdb -U admin family_app_restore
pg_restore -U admin -d family_app_restore family_app_2026-02-06_20-44-43.dump
```

