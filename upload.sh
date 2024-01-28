#curl -X POST -F "image=@/path/to/your/image.jpg" -F "caption=OptionalCaption" http://localhost:8080/upload

curl -X POST \
  -F "image=@./image.png" \
  -F "caption=OptionalCaption"  \
  -H "X-API-KEY: abc123" \
http://localhost:7171/upload


