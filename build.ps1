

# del generated code 
# Get-ChildItem .\enum\ -Recurse -Include *_gen.go | Remove-Item
# Get-ChildItem .\protocol_me\ -Recurse -Include *_gen.go | Remove-Item
# Remove-Item lib\melog\log_gen.go
# Remove-Item config/dataversion/dataversion_gen.go 

################################################################################
Set-Location lib
Write-Output "genlog -leveldatafile ./melog/melog.data -packagename melog "
genlog -leveldatafile ./melog/melog.data -packagename melog 
Set-Location ..

################################################################################
$PROTOCOL_GOS_VERSION=makesha256sum protocol_me/*.enum protocol_me/me_obj/protocol_*.go
Write-Output "Protocol GOS Version: ${PROTOCOL_GOS_VERSION}"
Write-Output "genprotocol -ver=${PROTOCOL_GOS_VERSION} -basedir=protocol_me -prefix=me -statstype=int"
genprotocol -ver="${PROTOCOL_GOS_VERSION}" -basedir=protocol_me -prefix=me -statstype=int
Set-Location protocol_me
goimports -w .
Set-Location ..

################################################################################
# generate enum
Write-Output "generate enums"

Set-Location enum
goimports -w .
Set-Location ..

$Data_VERSION=makesha256sum config/gameconst/*.go config/gamedata/*.go enum/*.enum
Write-Output "Data Version: ${Data_VERSION}"
mkdir -ErrorAction SilentlyContinue config/dataversion
Write-Output "package dataversion
const DataVersion = `"${Data_VERSION}`" 
" > config/dataversion/dataversion_gen.go 

