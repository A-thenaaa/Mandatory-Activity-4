# Mandatory-Activity-4 Readme
## Steps to run the program:
1. Clone the repo
2. Open 3 terminal windows
3. In the first window copy and paste this: go run server.go --id 0 --port :5050 --peer :5051,:5052 --peerid 1,2
4. In the second window copy and paste this: go run server.go --id 1 --port :5051 --peer :5050,:5052 --peerid 0,2
5. In the third window copy and paste this: go run server.go --id 2 --port :5052 --peer :5050,:5051 --peerid 0,1

NB! It is important that these three commands are run within 3 seconds of eachother!
