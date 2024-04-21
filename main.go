package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/api/iterator"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
)

const (
	projectID  = "image-upload-go"
	bucketName = "image_bucket_go"
)

type ClientUploader struct {
	cl         *storage.Client
	projectID  string
	bucketName string
	uploadPath string
}

// struct for bucket file names
type BucketFiles struct {
	Files []string `json:"files"`
}

var uploader *ClientUploader

func init() {
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "keys.json") // FILL IN WITH YOUR FILE PATH
	client, err := storage.NewClient(context.Background())
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	uploader = &ClientUploader{
		cl:         client,
		bucketName: bucketName,
		projectID:  projectID,
		uploadPath: "test-files/",
	}
}

func main() {
	r := gin.Default()
	r.SetFuncMap(template.FuncMap{
		"upper": strings.ToUpper,
	})
	r.Static("/assets", "./assets")
	r.LoadHTMLGlob("templates/*.html")

	// health check endpoint
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// home
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"content": "This is an index page...",
		})
	})

	r.GET("/about", func(c *gin.Context) {
		c.HTML(http.StatusOK, "about.html", gin.H{
			"content": "This is an about page...",
		})
	})

	r.GET("/images", func(c *gin.Context) {
		c.HTML(http.StatusOK, "images.html", gin.H{
			"content": "This is the images page...",
		})
	})

	r.GET("/bucket-files", func(c *gin.Context) {
		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		bucket := client.Bucket(bucketName)

		var files []string
		it := bucket.Objects(ctx, nil)
		for {
			attrs, err := it.Next()
			if err == nil {
				// No error, collect filename
				files = append(files, attrs.Name)
			} else if err == iterator.Done {
				// Reached the end of the iterator
				break
			} else {
				// Other error occurred
				log.Fatalf("it.Next() failed with error: %v", err)
			}
		}

		// Return the filenames as JSON
		c.JSON(200, BucketFiles{Files: files})
	})

	r.GET("/live-image-urls", func(c *gin.Context) {
		ctx := context.Background()

		// Initialize Google Cloud Storage client
		client, err := storage.NewClient(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to create client: %v", err)})
			return
		}
		defer client.Close()

		// Get bucket handle
		bucket := client.Bucket(bucketName)

		// List objects in the bucket
		it := bucket.Objects(ctx, nil)
		var imageURLs []string
		for {
			objAttrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to list objects: %v", err)})
				return
			}

			// Generate live URL for each object
			imageURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, objAttrs.Name)
			imageURLs = append(imageURLs, imageURL)
		}

		c.JSON(http.StatusOK, gin.H{"image_urls": imageURLs})
	})

	r.GET("/download/test-files/:filename", func(c *gin.Context) {
		filename := c.Param("filename")

		ctx := context.Background()
		client, err := storage.NewClient(ctx)
		if err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		bucket := client.Bucket(bucketName)
		obj := bucket.Object("test-files/" + filename)
		fmt.Println("Filename downloading", filename)

		// Open the file in the bucket
		reader, err := obj.NewReader(ctx)
		if err != nil {
			// Handle error (e.g., file not found)
			c.String(http.StatusNotFound, "File not found")
			return
		}
		defer reader.Close()

		// Set Content-Disposition header to prompt download
		c.Header("Content-Disposition", "attachment; filename="+filename)

		// Stream the file's contents to the response writer
		if _, err := io.Copy(c.Writer, reader); err != nil {
			// Handle error
			log.Printf("Failed to copy file contents: %v", err)
			return
		}
	})

	r.POST("/images", func(c *gin.Context) {
		f, err := c.FormFile("upload")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		blobFile, err := f.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		err = uploader.UploadFile(blobFile, f.Filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		c.JSON(200, gin.H{
			"message": "success",
		})
	})

	r.Run()
}

func (c *ClientUploader) UploadFile(file multipart.File, object string) error {
	ctx := context.Background()

	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()

	// Upload an object with storage.Writer.
	wc := c.cl.Bucket(c.bucketName).Object(c.uploadPath + object).NewWriter(ctx)
	if _, err := io.Copy(wc, file); err != nil {
		return fmt.Errorf("io.Copy: %v", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %v", err)
	}

	return nil
}

func getObjects(c *gin.Context) {
	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	bucket := client.Bucket("image_bucket_go")
	it := bucket.Objects(ctx, nil)
	fmt.Println("Bucket", it)
	var objects []string
	for {
		attr, err := it.Next()
		if err == storage.ErrBucketNotExist {
			fmt.Println("ErrBucketNotExist")
			break
		}
		if err != nil {
			log.Fatalf("Failed to fetch objects: %v", err)
		}
		objects = append(objects, attr.Name)
	}

	c.JSON(200, objects)
}
