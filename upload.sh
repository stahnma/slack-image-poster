#curl -X POST -F "image=@/path/to/your/image.jpg" -F "caption=OptionalCaption" http://localhost:8080/upload

file=`ls ~/Desktop/*.png ~/Desktop/*.jpeg |sort -R |tail -1`
echo $file

curl -X POST \
  -F "image=@$file" \
  -F "caption=`fortune`"  \
  -H "X-API-KEY: $API_KEY" \
http://localhost:7171/upload


