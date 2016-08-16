
PROTOC_LOCATION=`which protoc`

default: protos

protos: 
	 $(PROTOC_LOCATION) --go_out=plugins=grpc:. protos/*.proto

clean:
	echo "drop measurement moment" | influx --database testdb


.PHONY: protos
