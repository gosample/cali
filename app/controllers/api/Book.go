package api

import (
	"github.com/jiangmitiao/cali/app/models"
	"github.com/jiangmitiao/cali/app/rcali"
	"github.com/revel/revel"
	"io/ioutil"
	"net/url"
	"path"
	"strconv"
	"time"
)

type Book struct {
	*revel.Controller
}

func (c Book) Index() revel.Result {
	return c.RenderJSONP(c.Request.FormValue("callback"), models.NewOKApi())
}

//all books count
func (c Book) BooksCount() revel.Result {
	categoryid := rcali.ValueOrDefault(c.Request.FormValue("categoryid"), models.DefaultCaliCategory.Id)
	return c.RenderJSONP(
		c.Request.FormValue("callback"),
		models.NewOKApiWithInfo(bookService.QueryBooksCount(categoryid)))
}

//all books info
func (c Book) Books() revel.Result {
	categoryid := rcali.ValueOrDefault(c.Request.FormValue("categoryid"), models.DefaultCaliCategory.Id)
	limit, _ := strconv.Atoi(rcali.ValueOrDefault(c.Request.FormValue("limit"), rcali.ClassNumsStr))
	start, _ := strconv.Atoi(rcali.ValueOrDefault(c.Request.FormValue("start"), "0"))
	return c.RenderJSONP(
		c.Request.FormValue("callback"),
		models.NewOKApiWithInfo(bookService.QueryBooks(limit, start, categoryid)),
	)
}

//book's download
func (c Book) BookDown() revel.Result {
	//bytes := rcali.FILE(bookService.QueryBookFile(bookid))
	formatid := rcali.ValueOrDefault(c.Request.FormValue("formatid"), "0")
	if ok, format := formatService.GetById(formatid); ok {
		if f, err := bookService.QueryBookFile(format.Id); err == nil {
			user, _ := userService.GetLoginUser(c.Request.FormValue("session"))
			c.addDownloadRecord(format, user)
			return c.RenderBinary(f, format.Title+"-"+format.Author+"."+format.Format, revel.Attachment, time.Unix(format.UpdatedAt, 0))
			//return c.RenderFile(f, revel.Attachment)
		}
	}
	return c.RenderText("file is not exit")
}

func (c Book) addDownloadRecord(format models.CaliFormat, user models.UserInfo) {
	// add status to sys status
	key := time.Now().Format("20060102") + "-downsize"
	if status := sysStatusService.Get(key); status.Key != "" {
		value, _ := strconv.ParseInt(status.Value, 10, 0)
		value += format.UncompressedSize
		status.Value = strconv.FormatInt(value, 10)
		sysStatusService.UpdateStatus(status)
	} else {
		status = models.SysStatus{Key: key, Value: strconv.FormatInt(format.UncompressedSize, 10)}
		sysStatusService.AddSysStatus(status)
	}

	//add books download count
	_, book := bookService.QueryBook(format.CaliBook)
	book.DownloadCount += 1
	bookService.UpdateCaliBookDownload(book)

	//add format download count
	format.DownloadCount += 1
	formatService.UpdateCaliFormatDownload(format)

	//user download
	userService.AddDownload(user.Id, format.Id)
}

//query a book by bookid
func (c Book) Book() revel.Result {
	bookid := rcali.ValueOrDefault(c.Request.FormValue("bookid"), "0")
	if has, book := bookService.QueryBook(bookid); has {
		bookvo := models.CaliBookVo{CaliBook: book}
		bookvo.Formats = formatService.QueryByCaliBook(bookvo.Id)
		return c.RenderJSONP(
			c.Request.FormValue("callback"),
			models.NewOKApiWithInfo(bookvo),
		)
	} else {
		return c.RenderJSONP(
			c.Request.FormValue("callback"),
			models.NewErrorApiWithMessageAndInfo(c.Message("nofindbook"), nil),
		)
	}
}

//query a book's info from //https://developers.douban.com/wiki/?title=book_v2#get_isbn_book by bookid by bookname

//UPLOAD
func (c *Book) UploadBook() revel.Result {
	uploadpath, _ := rcali.GetUploadPath()
	tag := rcali.ValueOrDefault(c.Request.FormValue("tag"), "")
	if file, header, err := c.Request.FormFile("book"); err == nil {
		defer file.Close()
		b, _ := ioutil.ReadAll(file)
		tmpPath := path.Join(uploadpath, header.Filename)
		ioutil.WriteFile(tmpPath, b, 0755)

		if ok, format := bookService.UploadBookFormat(tmpPath, tag); ok {
			user, _ := userService.GetLoginUser(c.Request.FormValue("session"))
			c.addUploadRecord(format, user)
			return c.RenderJSON(models.NewOKApiWithMessageAndInfo("add book success", format))
		} else {
			return c.RenderJSON(models.NewErrorApiWithMessageAndInfo("add book error",nil))

		}
	} else {
		rcali.Logger.Debug("read file error :", err.Error())
		return c.RenderJSON(models.NewErrorApiWithMessageAndInfo(err.Error(),nil))
	}
	return c.RenderJSON(models.NewErrorApiWithMessageAndInfo("file read error",nil))
}

func (c Book) addUploadRecord(format models.CaliFormat, user models.UserInfo) {
	// add status to sys status
	key := time.Now().Format("20060102") + "-uploadsize"

	if status := sysStatusService.Get(key); status.Key != "" {
		value, _ := strconv.ParseInt(status.Value, 10, 0)
		value += format.UncompressedSize
		status.Value = strconv.FormatInt(value, 10)
		sysStatusService.UpdateStatus(status)
	} else {
		status = models.SysStatus{Key: key, Value: strconv.FormatInt(format.UncompressedSize, 10)}
		sysStatusService.AddSysStatus(status)
	}

	//user upload
	userService.AddUpload(user.Id, format.Id)
}

func (c *Book) UploadBookConfirm() revel.Result {
	//book
	book := bookService.GetBookOrInsertByTitleAndAuthor(rcali.ValueOrDefault(c.Request.FormValue("title"), ""), rcali.ValueOrDefault(c.Request.FormValue("author"), ""))
	book.DoubanId = rcali.ValueOrDefault(c.Request.FormValue("douban_id"), book.DoubanId)
	book.DoubanJson = rcali.GetDoubanInfoById(book.DoubanId)
	bookService.UpdateCaliBook(book)

	//category
	categoryId := rcali.ValueOrDefault(c.Request.FormValue("categoryid"), models.DefaultCaliCategory.Id)

	bookService.AddBookCategory(book.Id, categoryId)
	bookService.AddBookCategory(book.Id, models.DefaultCaliCategory.Id)

	//format
	formatid := rcali.ValueOrDefault(c.Request.FormValue("formatid"), "")
	formatService.UpdateBookid(formatid, book.Id)

	return c.RenderJSON(models.NewOKApi())
}

//SEARCH
func (c *Book) SearchCount() revel.Result {
	q, _ := url.QueryUnescape(c.Request.FormValue("q"))
	q = rcali.ValueOrDefault(q, "")
	if q == "" {
		return c.RenderJSONP(c.Request.FormValue("callback"), models.NewErrorApi())
	} else {
		return c.RenderJSONP(c.Request.FormValue("callback"), models.NewOKApiWithInfo(bookService.SearchBooksCount(q)))
	}
}

func (c *Book) Search() revel.Result {
	q, _ := url.QueryUnescape(c.Request.FormValue("q"))
	q = rcali.ValueOrDefault(q, "")
	limit, _ := strconv.Atoi(rcali.ValueOrDefault(c.Request.FormValue("limit"), rcali.ClassNumsStr))
	start, _ := strconv.Atoi(rcali.ValueOrDefault(c.Request.FormValue("start"), "0"))
	if q == "" {
		return c.RenderJSONP(c.Request.FormValue("callback"), models.NewErrorApi())
	} else {
		return c.RenderJSONP(c.Request.FormValue("callback"), models.NewOKApiWithInfo(bookService.SearchBooks(q, limit, start)))
	}
}
