# Резервные копии: что именно сохраняется

Контейнер можно пересоздать из образа. Volume хранит данные, которые нельзя автоматически получить из Git. Резервная копия на том же VPS не защищает от удаления VPS, отказа диска или взлома.

## Управляющая база и конфигурация

В папке DeployFox на Ubuntu:

```bash
sudo bash infrastructure/scripts/backup.sh
sudo ls -l backups
```

Получится каталог с датой UTC. В нём deployfox.dump, список содержимого, compose.yml, Caddyfile, .env и PEM-ключ при наличии. Файлы закрыты правами umask 077. SHA256SUMS позволяет выявить случайное повреждение, но не подмену злоумышленником, способным изменить и архив, и контрольные суммы.

Для локального стека в Linux/Git Bash:

```bash
COMPOSE_FILE=docker-compose.local.yml bash infrastructure/scripts/backup.sh
```

На Windows PowerShell не перенаправляйте бинарный pg_dump через старый PowerShell: используйте Git Bash/WSL или копирование файла из контейнера. Локальный secrets volume входит в копию отдельным файлом.

## Экспорт за пределы VPS

Пример PowerShell, замените каталог даты на реально созданный:

```powershell
scp -i "$env:USERPROFILE\.ssh\deployfox_vps" -r deploy@203.0.113.10:/home/deploy/apps/deployfox/backups/КАТАЛОГ_ДАТЫ .
```

Если копия создана root и не читается deploy, на VPS из папки DeployFox выполните, заменив КАТАЛОГ_ДАТЫ настоящим именем:

```bash
sudo tar -czf /home/deploy/deployfox-backup.tar.gz -C backups КАТАЛОГ_ДАТЫ
sudo chown deploy:deploy /home/deploy/deployfox-backup.tar.gz
sudo chmod 600 /home/deploy/deployfox-backup.tar.gz
```

Затем в PowerShell:

```powershell
scp -i "$env:USERPROFILE\.ssh\deployfox_vps" deploy@203.0.113.10:/home/deploy/deployfox-backup.tar.gz .
```

После проверки скачанного архива на VPS можно удалить **только временный архив**: `rm /home/deploy/deployfox-backup.tar.gz`. Не открывайте `.env` всем через chmod 777. Храните копию в зашифрованном внешнем хранилище; настройка S3/автоматической выгрузки в этой версии не реализована.

## Управляемые базы и volumes

Кнопка backup делает PostgreSQL custom dump, MySQL SQL dump или Redis RDB в backups/managed с SHA256. Для PostgreSQL/MySQL доступен подтверждаемый restore в панели; PostgreSQL проверен реальным восстановлением, MySQL пока нет. Redis RDB проверен восстановлением ключа в отдельный экземпляр без AOF; restore Redis в панели не реализован. Общий backup volumes, routes и сертификатов пока не завершён. Отдельно сохраните пользовательские данные по инструкции соответствующей СУБД. Не копируйте активно изменяющийся каталог базы как обычные файлы: такая копия может быть несогласованной.

## Обязательная проверка восстановления

Восстанавливайте на отдельном VPS/стеке, не поверх работающего единственного сервера. Следуйте [главе 19](19-restore-server.md). До успешного пробного восстановления считайте backup **непроверенным**. Скрипт проверяет структуру pg_restore --list и SHA256, но это не заменяет открытие восстановленного приложения и чтение его данных.
