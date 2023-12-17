package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/gorilla/mux"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Blog struct {
	ID       int       `json:"id"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	Author   string    `json:"author"`
	Likes    int       `json:"likes"`
	Comments []Comment `json:"comments"`
}

type Comment struct {
	Author  string `json:"author"`
	Content string `json:"content"`
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

var (
	client   *mongo.Client
	blogColl *mongo.Collection
	userColl *mongo.Collection
	blogsMu  sync.RWMutex
	usersMu  sync.RWMutex
	currentID int
)

func init() {
	connectionString := os.Getenv("DB_CREDENTIALS")

	// Connect to MongoDB Atlas
	clientOptions := options.Client().ApplyURI(connectionString)
	var err error
	client, err = mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Connected to MongoDB Atlas!")

	// Initialize blog and user collections
	blogColl = client.Database("blogdb").Collection("blogs")
	userColl = client.Database("blogdb").Collection("users")
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	usersMu.RLock()
	defer usersMu.RUnlock()

	var result User
	err = userColl.FindOne(context.TODO(), bson.M{"username": user.Username, "password": user.Password}).Decode(&result)
	if err == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
}

func signupHandler(w http.ResponseWriter, r *http.Request) {
	var user User
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// Check if the username is already taken
	usersMu.RLock()
	defer usersMu.RUnlock()

	existingUser := User{}
	err = userColl.FindOne(context.TODO(), bson.M{"username": user.Username}).Decode(&existingUser)
	if err == nil {
		http.Error(w, "Username is already taken", http.StatusConflict)
		return
	}

	// Create a new user
	_, err = userColl.InsertOne(context.TODO(), user)
	if err != nil {
		http.Error(w, "Error creating user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func getBlogsHandler(w http.ResponseWriter, r *http.Request) {
	blogsMu.RLock()
	defer blogsMu.RUnlock()

	var result []Blog
	cursor, err := blogColl.Find(context.TODO(), bson.D{})
	if err != nil {
		log.Fatal(err)
	}

	defer cursor.Close(context.TODO())

	for cursor.Next(context.TODO()) {
		var blog Blog
		err := cursor.Decode(&blog)
		if err != nil {
			log.Fatal(err)
		}
		result = append(result, blog)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func createBlogHandler(w http.ResponseWriter, r *http.Request) {
	var blog Blog
	err := json.NewDecoder(r.Body).Decode(&blog)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	blogsMu.Lock()
	defer blogsMu.Unlock()

	currentID++
	blog.ID = currentID

	_, err = blogColl.InsertOne(context.TODO(), blog)
	if err != nil {
		log.Fatal(err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(blog)
}

func deleteBlogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid blog ID", http.StatusBadRequest)
		return
	}

	blogsMu.Lock()
	defer blogsMu.Unlock()

	_, err = blogColl.DeleteOne(context.TODO(), bson.M{"id": id})
	if err != nil {
		http.Error(w, "Blog not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}


func likeBlogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid blog ID", http.StatusBadRequest)
		return
	}

	blogsMu.Lock()
	defer blogsMu.Unlock()

	filter := bson.M{"id": id}
	update := bson.M{"$inc": bson.M{"likes": 1}}
	_, err = blogColl.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		http.Error(w, "Blog not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func commentBlogHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid blog ID", http.StatusBadRequest)
		return
	}

	var comment Comment
	err = json.NewDecoder(r.Body).Decode(&comment)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	blogsMu.Lock()
	defer blogsMu.Unlock()

	filter := bson.M{"id": id}
	update := bson.M{"$push": bson.M{"comments": comment}}
	_, err = blogColl.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		http.Error(w, "Blog not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func main() {
	router := mux.NewRouter()

	router.HandleFunc("/api/users", loginHandler).Methods("POST")
	router.HandleFunc("/api/users", signupHandler).Methods("POST")
	router.HandleFunc("/api/blogs", getBlogsHandler).Methods("GET")
	router.HandleFunc("/api/blogs", createBlogHandler).Methods("POST")
	router.HandleFunc("/api/blogs/{id}/like", likeBlogHandler).Methods("POST")
	router.HandleFunc("/api/blogs/{id}/comment", commentBlogHandler).Methods("POST")
	router.HandleFunc("/api/blogs/{id}", deleteBlogHandler).Methods("DELETE")

	log.Fatal(http.ListenAndServe(":3030", router))
}
