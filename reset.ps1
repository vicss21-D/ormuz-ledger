Write-Host "Parando contentores e removendo volumes órfãos..."
docker compose down -v

Write-Host "Limpando o estado do LevelDB (Aplicação)..."
Remove-Item -Recurse -Force .\deployments\data\* -ErrorAction SilentlyContinue

Write-Host "Limpando o histórico de blocos do CometBFT (Consenso)..."
Remove-Item -Recurse -Force .\deployments\config\*\data\* -ErrorAction SilentlyContinue

Write-Host "Recriando estados zerados para os validadores (sem BOM)..."
$nations = @("br", "fr", "uk", "us")
$jsonContent = '{
  "height": "0",
  "round": 0,
  "step": 0
}'

foreach ($nation in $nations) {
    $path = ".\deployments\config\$nation\data"
    New-Item -ItemType Directory -Force -Path $path | Out-Null
    
    # O Encoding ASCII previne o erro de parse no Linux/Docker
    Set-Content -Path "$path\priv_validator_state.json" -Value $jsonContent -Encoding ASCII
    Write-Host " - Nação $nation pronta."
}

Write-Host "Ambiente perfeitamente limpo e pronto para o Bloco 0!"