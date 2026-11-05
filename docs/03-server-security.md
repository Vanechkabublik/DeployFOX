# Безопасность первого входа

**Не закрывайте первое SSH-окно, пока не проверите второе соединение.** Если что-то сломается, исправьте конфигурацию через первое окно или web console провайдера. Команды ниже предполагают первоначальный вход root и стандартный SSH-порт 22.

## Обновления и отдельный пользователь

В Ubuntu:

```bash
apt update
apt upgrade
adduser deploy
usermod -aG sudo deploy
```

`apt` устанавливает пакеты. Прочитайте список перед подтверждением обновления. `adduser` запросит новый пароль; остальные необязательные поля можно пропустить Enter. `sudo` позволяет выполнять административные команды с подтверждением паролем.

## Создание ключа на вашем компьютере

В **новом локальном PowerShell**, а не на VPS:

```powershell
ssh-keygen -t ed25519 -f "$env:USERPROFILE\.ssh\deployfox_vps"
Get-Content "$env:USERPROFILE\.ssh\deployfox_vps.pub"
```

Выберите парольную фразу для защиты приватного ключа. Файл `.pub` публичный, его можно скопировать на сервер; файл без `.pub` приватный, храните его только у себя и в защищённой резервной копии.

В первом окне Ubuntu:

```bash
install -d -m 700 -o deploy -g deploy /home/deploy/.ssh
nano /home/deploy/.ssh/authorized_keys
```

Вставьте целиком одну строку из `.pub`. В nano сохраните Ctrl+O, Enter, выйдите Ctrl+X. Затем:

```bash
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 600 /home/deploy/.ssh/authorized_keys
```

В новом PowerShell проверьте:

```powershell
ssh -i "$env:USERPROFILE\.ssh\deployfox_vps" deploy@203.0.113.10
```

В новом SSH-окне выполните `sudo -v`: пароль пользователя deploy должен дать доступ к sudo.

## Отключение паролей SSH — только после успешного теста

В первом окне создайте файл:

```bash
nano /etc/ssh/sshd_config.d/00-deployfox.conf
```

Содержимое:

```text
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
```

Проверьте синтаксис и **эффективные** значения (другие файлы могут влиять на настройки):

```bash
sshd -t
sshd -T | grep -E 'passwordauthentication|kbdinteractiveauthentication|permitrootlogin|pubkeyauthentication'
systemctl reload ssh
```

Ещё раз откройте третье соединение deploy по ключу. Только после этого закрывайте первое. При ошибке удалите или исправьте созданный файл через оставшееся окно, снова проверьте `sshd -t` и перезагрузите настройки.

## Сетевой экран и автоматические обновления

```bash
apt install ufw unattended-upgrades
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp
ufw enable
ufw status verbose
dpkg-reconfigure -plow unattended-upgrades
```

Если SSH работает на другом порту, разрешите **этот порт до включения UFW**. В firewall провайдера также откройте SSH, 80 и 443. Не открывайте PostgreSQL 5432, Redis 6379, MySQL 3306 и Docker API 2375.

Docker-публикации портов могут обходить правила UFW. Поэтому Compose публикует только Caddy, а базы остаются без `ports`. Подробнее: [официальная документация Docker](https://docs.docker.com/engine/install/ubuntu/#firewall-limitations).
