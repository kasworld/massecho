
# del generated code 
# Get-ChildItem .\enum\ -Recurse -Include *_gen.go | Remove-Item
# Get-ChildItem .\protocol_me\ -Recurse -Include *_gen.go | Remove-Item
# Remove-Item config/dataversion/dataversion_gen.go 

################################################################################
$PROTOCOL_GOS_VERSION=makesha256sum protocol_me/*.enum protocol_me/me_obj/protocol_*.go
Write-Output "Protocol GOS Version: ${PROTOCOL_GOS_VERSION}"
Write-Output "genprotocol -ver=${PROTOCOL_GOS_VERSION} -basedir=protocol_me -prefix=me -statstype=int"
genprotocol -ver="${PROTOCOL_GOS_VERSION}" -basedir=protocol_me -prefix=me -statstype=int
goimports -w protocol_me

################################################################################
msgp -file protocol_me/me_obj/obj.go -o protocol_me/me_obj/obj_gen.go -tests=0 

################################################################################
# generate enum
Write-Output "generate enums"
goimports -w enum

$Data_VERSION=makesha256sum config/gameconst/*.go config/gamedata/*.go enum/*.enum
Write-Output "Data Version: ${Data_VERSION}"
mkdir -ErrorAction SilentlyContinue config/dataversion
Write-Output "package dataversion
const DataVersion = `"${Data_VERSION}`" 
" > config/dataversion/dataversion_gen.go 

################################################################################
$DATESTR=Get-Date -UFormat '+%Y-%m-%dT%H:%M:%S%Z:00'
$GITSTR=git rev-parse HEAD
################################################################################
# build bin

$BIN_DIR="bin"
$SRC_DIR="."

mkdir -ErrorAction SilentlyContinue "${BIN_DIR}"

$BUILD_VER="${DATESTR}_${GITSTR}_release_windows"
Write-Output "Build Version: ${BUILD_VER}"
Write-Output ${BUILD_VER} > ${BIN_DIR}/BUILD_windows
go build -o "${BIN_DIR}\server.exe" -ldflags "-X main.Ver=${BUILD_VER}" "${SRC_DIR}\server.go"
go build -o "${BIN_DIR}\multiclient.exe" -ldflags "-X main.Ver=${BUILD_VER}" "${SRC_DIR}\multiclient.go"

$BUILD_VER="${DATESTR}_${GITSTR}_release_linux"
Write-Output "Build Version: ${BUILD_VER}"
Write-Output ${BUILD_VER} > ${BIN_DIR}/BUILD_linux
$env:GOOS="linux" 
go build -o "${BIN_DIR}\server" -ldflags "-X main.Ver=${BUILD_VER}" "${SRC_DIR}\server.go"
go build -o "${BIN_DIR}\multiclient" -ldflags "-X main.Ver=${BUILD_VER}" "${SRC_DIR}\multiclient.go"
$env:GOOS=""
