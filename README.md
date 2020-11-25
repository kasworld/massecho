# massive echo server


go 언어로 만든 서버에서 64K connection 만들어 보기
재미삼아 만들어본 프로젝트입니다. 

 https://github.com/kasworld/massecho

tcp연결이나 gorilla websocket을 사용해서 테스트 가능합니다. 



테스트 전에 준비해야 할것이 대규모의 connection을 테스트 하기위해선 몇가지의 제한을 넘어야 합니다. 

1. OS에서 열수있는 파일수의 제한 

2. OS에서 client socket 제한 

윈도우즈가 주 개발 환경인 상황에서 linux 테스트를 위해 설치한 WSL2 와 virtualbox의 ubuntu에서는 openfile 수의 제한을 올릴수가 없습니다. 

( 아니면 방법을 못찾은 듯)

ubuntu를 설치한 노트북에선 같은 방법으로 잘되는 것으로 봐선 그냥 막아둔듯 합니다. 

linux에서 파일 수 제한을 올리는 방법은 

/etc/security/limits.conf 내용에 

* soft     nofile         999999   

* hard     nofile         999999

root soft     nofile         999999   

root hard     nofile         999999

를 추가하면 됩니다. 


llinux에서 client socket수를 늘리는 것은  

파일 이름은 적당히 정해주면 됩니다. readme에 의하면 local.conf를 추천 하더군요. 

/etc/sysctl.d/local.conf 파일 내에 

net.ipv4.ip_local_port_range = 1024    65535

net.core.somaxconn = 65535

fs.file-max = 999999


windows에서는 https://docs.microsoft.com/en-us/troubleshoot/windows-client/networking/connect-tcp-greater-than-5000-error-wsaenobufs-10055

를 참고해서

레지스트리 에디터로 

HKEY_LOCAL_MACHINE\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters

Value Name: MaxUserPort
Value Type: DWORD Value data: 65534

를 설정하면 됩니다.



linux / windows 공히 재시작은 필수 입니다. 



2020-11-25 업데이트 

AMD Ryzen 3700X (8c16t) 에서 websocket / tcp 연결 모두 64000 connection 성공했습니다.

이상하게 제가 만든 tcp connection이 gorilla websocket 보다 느려서 자존심 상해 하고 있었는데 오늘 원인을 찾아 수정했습니다. 

이젠  tcp connection이 아주 조금 더 빠릅니다. ^^ 

그밖에도 genprotocol이 만드는 코드쪽에 성능과 사용 편의성 향상을 위한 코드 수정들이 있었습니다. 