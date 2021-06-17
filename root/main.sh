#!/usr/bin/env bash

echo "== VARIAVEIS DE AMBIENTE =="
echo "=> exe: '$exe'"
echo "=> main work dir: '$cwd'"
echo "=> service dir: '$service_dir'"
echo "=> static dir: '$static_dir'"
echo "=> porta de desmontagem do FS de root/service: $umnt_service_port"
echo "=> porta de desmontagem do FS de root/static: $umnt_static_port"
echo "=> porta do servidor de processos: $start_child_port"

# roda um script /bin/sh em background a partir da entrada padrao
start_script()
{
  "$exe" "!script" $start_child_port
}

# roda um programa em background a partir de argumentos
start_cmd()
{
  "$exe" "!cmd" $start_child_port "$@"
}

# forca a desmontagem de root/service
umount_service()
{
  nc -z -v localhost $umnt_service_port
}

# forca a desmontagem de root/static
umount_static()
{
  nc -z -v localhost $umnt_static_port
}

# roda um programa em background
pid=$(start_cmd echo CMD DONE)
echo "=> cmd pid: $pid"

# roda um script /bin/sh em background

pid2=$(echo 'echo SCRIPT PWD: $(pwd);date;sleep 10;echo SCRIPT DONE' | start_script)
echo "=> script pid: $pid2"

# inicia o servidor nodejs em background
(
  cd "$service_dir"
  pid3=$(start_cmd nodejs "$service_dir/hello_server.js")
  echo "=> nodejs server pid: $pid3"
)

sleep 2

# desmounta o root/service
umount_service