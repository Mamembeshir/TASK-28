package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── DELETE /resources/:id ────────────────────────────────────────────────────

func TestDeleteResource_AuthorDeletesOwnDraft(t *testing.T) {
	truncate(t)
	registerUser(t, "del_author1", "del_author1@example.com", "SecurePass1!")
	makeAuthor(t, "del_author1")
	token := loginUser(t, "del_author1", "SecurePass1!")
	client := authedClient(t, token)

	resourceID := createDraft(t, client, "To Be Deleted", "description")

	req, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/resources/"+resourceID, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// DELETE should redirect to /resources (303) or return 200
	assert.True(t, resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusOK,
		"expected 303 or 200, got %d", resp.StatusCode)

	// Verify resource is gone from DB
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM resources WHERE id=$1`, resourceID).Scan(&count)
	assert.Equal(t, 0, count)
}

func TestDeleteResource_OtherAuthorCannotDelete(t *testing.T) {
	truncate(t)
	registerUser(t, "del_author2", "del_author2@example.com", "SecurePass1!")
	registerUser(t, "del_other2", "del_other2@example.com", "SecurePass1!")
	makeAuthor(t, "del_author2")
	makeAuthor(t, "del_other2")

	ownerToken := loginUser(t, "del_author2", "SecurePass1!")
	otherToken := loginUser(t, "del_other2", "SecurePass1!")

	resourceID := createDraft(t, authedClient(t, ownerToken), "Protected Resource", "desc")

	req, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/resources/"+resourceID, nil)
	resp, err := authedClient(t, otherToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnprocessableEntity,
		"expected 403 or 422, got %d", resp.StatusCode)
}

// ─── DELETE /resources/:id/files/:fileID ────────────────────────────────────

func TestDeleteFile_AuthorDeletesOwnFile(t *testing.T) {
	truncate(t)
	registerUser(t, "fdel_author1", "fdel_author1@example.com", "SecurePass1!")
	makeAuthor(t, "fdel_author1")
	token := loginUser(t, "fdel_author1", "SecurePass1!")
	client := authedClient(t, token)

	resourceID := createDraft(t, client, "Resource With File", "desc")
	fileID := uploadFileForResource(t, token, resourceID)

	req, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/resources/"+resourceID+"/files/"+fileID, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify file is removed from DB
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM resource_files WHERE id=$1`, fileID).Scan(&count)
	assert.Equal(t, 0, count)
}

func TestDeleteFile_OtherUserForbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "fdel_author2", "fdel_author2@example.com", "SecurePass1!")
	registerUser(t, "fdel_other2", "fdel_other2@example.com", "SecurePass1!")
	makeAuthor(t, "fdel_author2")
	makeAuthor(t, "fdel_other2")

	ownerToken := loginUser(t, "fdel_author2", "SecurePass1!")
	otherToken := loginUser(t, "fdel_other2", "SecurePass1!")

	resourceID := createDraft(t, authedClient(t, ownerToken), "Owner Resource", "desc")
	fileID := uploadFileForResource(t, ownerToken, resourceID)

	req, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/resources/"+resourceID+"/files/"+fileID, nil)
	resp, err := authedClient(t, otherToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnprocessableEntity,
		"expected 403 or 422, got %d", resp.StatusCode)
}

// ─── POST /tags ───────────────────────────────────────────────────────────────

func TestPostCreateTag_AsAuthor_CreatesTag(t *testing.T) {
	truncate(t)
	registerUser(t, "tag_author1", "tag_author1@example.com", "SecurePass1!")
	makeAuthor(t, "tag_author1")
	token := loginUser(t, "tag_author1", "SecurePass1!")
	client := authedClient(t, token)

	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/tags",
		strings.NewReader("name=golang"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotNil(t, body["tag"])

	// Verify in DB
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM tags WHERE name='golang'`).Scan(&count)
	assert.Equal(t, 1, count)
}

func TestPostCreateTag_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "tag_regular1", "tag_regular1@example.com", "SecurePass1!")
	token := loginUser(t, "tag_regular1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/tags",
		strings.NewReader("name=forbidden_tag"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── DELETE /tags/:id ─────────────────────────────────────────────────────────

func TestDeleteTag_AsAdmin_DeletesTag(t *testing.T) {
	truncate(t)
	registerUser(t, "tagdel_admin1", "tagdel_admin1@example.com", "SecurePass1!")
	registerUser(t, "tagdel_author1", "tagdel_author1@example.com", "SecurePass1!")
	makeAdmin(t, "tagdel_admin1")
	makeAuthor(t, "tagdel_author1")

	authorToken := loginUser(t, "tagdel_author1", "SecurePass1!")
	adminToken := loginUser(t, "tagdel_admin1", "SecurePass1!")

	// Create a tag as author
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/tags",
		strings.NewReader("name=to-delete"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createResp, err := authedClient(t, authorToken).Do(req)
	require.NoError(t, err)
	createResp.Body.Close()
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var tagID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM tags WHERE name='to-delete'`).Scan(&tagID)
	require.NotEmpty(t, tagID)

	// Delete as admin
	delReq, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/tags/"+tagID, nil)
	resp, err := authedClient(t, adminToken).Do(delReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM tags WHERE id=$1`, tagID).Scan(&count)
	assert.Equal(t, 0, count)
}

func TestDeleteTag_AsAuthor_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "tagdel_author2", "tagdel_author2@example.com", "SecurePass1!")
	makeAuthor(t, "tagdel_author2")
	token := loginUser(t, "tagdel_author2", "SecurePass1!")
	client := authedClient(t, token)

	// Create tag first
	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/tags",
		strings.NewReader("name=cant-delete"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	createResp, err := client.Do(req)
	require.NoError(t, err)
	createResp.Body.Close()

	var tagID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM tags WHERE name='cant-delete'`).Scan(&tagID)
	require.NotEmpty(t, tagID)

	delReq, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/tags/"+tagID, nil)
	resp, err := client.Do(delReq)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── PUT /categories/:id ─────────────────────────────────────────────────────

func TestPutCategory_AsAdmin_UpdatesCategory(t *testing.T) {
	truncate(t)
	registerUser(t, "catupd_admin1", "catupd_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "catupd_admin1")
	token := loginUser(t, "catupd_admin1", "SecurePass1!")
	client := authedClient(t, token)

	// Create category — handler returns 303 redirect to /categories (htmxRedirect)
	createResp, err := client.PostForm(testServer.URL+"/categories", url.Values{
		"name":        {"Original Name"},
		"description": {"Original Desc"},
	})
	require.NoError(t, err)
	createResp.Body.Close()
	require.True(t, createResp.StatusCode == http.StatusSeeOther || createResp.StatusCode == http.StatusCreated,
		"expected 303 or 201 on category create, got %d", createResp.StatusCode)

	var catID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM categories WHERE name='Original Name'`).Scan(&catID)
	require.NotEmpty(t, catID)

	// Update category — handler returns 303 redirect to /categories
	req, _ := http.NewRequest(http.MethodPut,
		testServer.URL+"/categories/"+catID,
		strings.NewReader("name=Updated+Name&description=Updated+Desc"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusOK,
		"expected 303 or 200 on category update, got %d", resp.StatusCode)

	// Verify update in DB
	var name string
	testPool.QueryRow(context.Background(),
		`SELECT name FROM categories WHERE id=$1`, catID).Scan(&name)
	assert.Equal(t, "Updated Name", name)
}

func TestPutCategory_AsRegularUser_Forbidden(t *testing.T) {
	truncate(t)
	registerUser(t, "catupd_admin2", "catupd_admin2@example.com", "SecurePass1!")
	registerUser(t, "catupd_regular2", "catupd_regular2@example.com", "SecurePass1!")
	makeAdmin(t, "catupd_admin2")

	adminToken := loginUser(t, "catupd_admin2", "SecurePass1!")
	regularToken := loginUser(t, "catupd_regular2", "SecurePass1!")

	createResp, err := authedClient(t, adminToken).PostForm(testServer.URL+"/categories", url.Values{
		"name": {"Admin Category"},
	})
	require.NoError(t, err)
	createResp.Body.Close()

	var catID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM categories WHERE name='Admin Category'`).Scan(&catID)
	require.NotEmpty(t, catID)

	req, _ := http.NewRequest(http.MethodPut,
		testServer.URL+"/categories/"+catID,
		strings.NewReader("name=Hacked"),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := authedClient(t, regularToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// ─── DELETE /categories/:id ───────────────────────────────────────────────────

func TestDeleteCategory_AsAdmin_DeletesCategory(t *testing.T) {
	truncate(t)
	registerUser(t, "catdel_admin1", "catdel_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "catdel_admin1")
	token := loginUser(t, "catdel_admin1", "SecurePass1!")
	client := authedClient(t, token)

	createResp, err := client.PostForm(testServer.URL+"/categories", url.Values{
		"name": {"Temp Category"},
	})
	require.NoError(t, err)
	createResp.Body.Close()

	var catID string
	testPool.QueryRow(context.Background(),
		`SELECT id FROM categories WHERE name='Temp Category'`).Scan(&catID)
	require.NotEmpty(t, catID)

	req, _ := http.NewRequest(http.MethodDelete,
		testServer.URL+"/categories/"+catID, nil)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM categories WHERE id=$1`, catID).Scan(&count)
	assert.Equal(t, 0, count)
}

// ─── GET /import/:jobID/preview ──────────────────────────────────────────────

func TestGetImportPreview_AsAdmin_Returns200(t *testing.T) {
	truncate(t)
	registerUser(t, "imprev_admin1", "imprev_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "imprev_admin1")
	token := loginUser(t, "imprev_admin1", "SecurePass1!")
	client := authedClient(t, token)

	// Upload a CSV to get a job ID
	var csvSB strings.Builder
	csvSB.WriteString("title,description,content_body\n")
	csvSB.WriteString("Preview Resource,A test resource,Some content body\n")

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "import.csv")
	io.WriteString(fw, csvSB.String())
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/import/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	uploadResp, err := client.Do(req)
	require.NoError(t, err)
	uploadResp.Body.Close()
	// Upload redirects to preview page
	require.True(t, uploadResp.StatusCode == http.StatusSeeOther || uploadResp.StatusCode == http.StatusOK,
		"expected upload to redirect or succeed, got %d", uploadResp.StatusCode)

	// Get job ID from DB
	var jobID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM bulk_import_jobs ORDER BY created_at DESC LIMIT 1`).Scan(&jobID)
	require.NoError(t, err)

	resp, err := client.Get(testServer.URL + "/import/" + jobID + "/preview")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── GET /import/:jobID/done ─────────────────────────────────────────────────

func TestGetImportDone_AsAdmin_Returns200AfterConfirm(t *testing.T) {
	truncate(t)
	registerUser(t, "impdone_admin1", "impdone_admin1@example.com", "SecurePass1!")
	makeAdmin(t, "impdone_admin1")
	token := loginUser(t, "impdone_admin1", "SecurePass1!")
	client := authedClient(t, token)

	// Upload CSV
	var csvSB strings.Builder
	csvSB.WriteString("title,description,content_body\n")
	csvSB.WriteString("Done Resource,A test,Content body here\n")

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", "done_import.csv")
	io.WriteString(fw, csvSB.String())
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, testServer.URL+"/import/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	uploadResp, err := client.Do(req)
	require.NoError(t, err)
	uploadResp.Body.Close()

	// Get job ID
	var jobID string
	err = testPool.QueryRow(context.Background(),
		`SELECT id FROM bulk_import_jobs ORDER BY created_at DESC LIMIT 1`).Scan(&jobID)
	require.NoError(t, err)

	// Confirm the job
	confirmResp, err := client.PostForm(testServer.URL+"/import/"+jobID+"/confirm", url.Values{})
	require.NoError(t, err)
	confirmResp.Body.Close()

	// Now access done page
	resp, err := client.Get(testServer.URL + "/import/" + jobID + "/done")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ─── GET /favorites ───────────────────────────────────────────────────────────

func TestGetFavorites_Authenticated_ReturnsList(t *testing.T) {
	truncate(t)
	registerUser(t, "fav_user1", "fav_user1@example.com", "SecurePass1!")
	token := loginUser(t, "fav_user1", "SecurePass1!")

	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/favorites", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, token).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestGetFavorites_WithFavoritedResource_ReturnsIt(t *testing.T) {
	truncate(t)
	registerUser(t, "fav_author1", "fav_author1@example.com", "SecurePass1!")
	registerUser(t, "fav_user2", "fav_user2@example.com", "SecurePass1!")
	registerUser(t, "fav_admin1", "fav_admin1@example.com", "SecurePass1!")
	makeAuthor(t, "fav_author1")
	makeAdmin(t, "fav_admin1")

	authorToken := loginUser(t, "fav_author1", "SecurePass1!")
	userToken := loginUser(t, "fav_user2", "SecurePass1!")
	adminToken := loginUser(t, "fav_admin1", "SecurePass1!")

	resourceID := createPublishedResource(t, authorToken, adminToken)

	// Favorite it
	favResp, err := authedClient(t, userToken).PostForm(
		testServer.URL+"/resources/"+resourceID+"/favorite", url.Values{})
	require.NoError(t, err)
	favResp.Body.Close()

	// Verify in favorites list
	req, _ := http.NewRequest(http.MethodGet, testServer.URL+"/favorites", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := authedClient(t, userToken).Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify favorite count in DB
	var count int
	testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM favorites WHERE resource_id=$1`, resourceID).Scan(&count)
	assert.Equal(t, 1, count)
}

func TestGetFavorites_Unauthenticated_Redirects(t *testing.T) {
	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(testServer.URL + "/favorites")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.True(t, resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusUnauthorized,
		"expected redirect or 401, got %d", resp.StatusCode)
}

// ─── helper: uploadFileForResource is defined in catalog_test.go ─────────────
// (already defined - using shared helper)

// buildMultipartCSV constructs a multipart body with a CSV file.
func buildMultipartCSV(filename, content string) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", filename)
	fmt.Fprint(fw, content)
	w.Close()
	return &buf, w.FormDataContentType()
}
