package controllers

import (
    "crypto/md5"
    "fmt"
    "github.com/QLeelulu/goku"
    "github.com/QLeelulu/goku/form"
    "github.com/QLeelulu/ohlala/golink"
    "github.com/QLeelulu/ohlala/golink/models"
    "github.com/QLeelulu/ohlala/golink/utils"
    "html/template"
    "net/http"
    "strconv"
    "strings"
    "time"
)

/**
 * form
 */
func createLoginForm() *form.Form {
    // defined the field
    email := form.NewEmailField("email", "Email", true).
        Error("invalid", "Email地址错误").
        Error("require", "Email地址必须填写").Field()

    pwd := form.NewCharField("pwd", "密码", true).Min(6).Max(30).
        Error("required", "密码必须填写").
        Error("range", "密码长度必须在{0}到{1}之间").Field()

    remeber_me := form.NewCharField("remeber_me", "记住我", false)

    // add the fields to a form
    form := form.NewForm(email, pwd, remeber_me)
    return form
}

func createRegForm() *form.Form {

    key := form.NewCharField("key", "邀请码", true).
        Error("required", "邀请码必须填写").Field()

    email := form.NewEmailField("email", "Email", true).
        Error("invalid", "Email地址错误").
        Error("required", "Email地址必须填写").Field()

    pwd := form.NewCharField("pwd", "密码", true).Min(6).Max(30).
        Error("required", "密码必须填写").
        Error("range", "密码长度必须在{0}到{1}之间").Field()

    name := form.NewCharField("name", "昵称", true).Min(2).Max(15).
        Error("required", "昵称必须填写").
        Error("range", "昵称长度必须在{0}到{1}之间").Field()

    repwd := form.NewCharField("repwd", "确认密码", true).Min(6).Max(30).
        Error("required", "确认密码必须填写").
        Error("range", "密码长度必须在{0}到{1}之间").Field()

    // add the fields to a form
    form := form.NewForm(key, email, name, pwd, repwd)
    return form
}

//为别的平台用户写cookie
func setCookieForOtherPlatformUser(userId int64, email string, seconds int, ctx *goku.HttpContext) {
    //注册成功,写cookie
    now := time.Now()
    h := md5.New()
    h.Write([]byte(fmt.Sprintf("%v-%v", email, now.Unix())))
    ticket := fmt.Sprintf("%x_%v", h.Sum(nil), now.Unix())
    expires := now.Add(time.Duration(seconds) * time.Second)
    redisClient := models.GetRedis()
    defer redisClient.Quit()
    err := redisClient.Set(ticket, userId)
    if err != nil {
        goku.Logger().Errorln(err.Error())
    } else {
        _, err = redisClient.Expireat(ticket, expires.Unix())
        if err != nil {
            goku.Logger().Errorln(err.Error())
        }
        c := &http.Cookie{
            Name:     "_glut",
            Value:    ticket,
            Expires:  expires,
            Path:     "/",
            HttpOnly: true,
        }
        ctx.SetCookie(c)
    }
}

/**
 * Controller: user
 */
var _ = goku.Controller("user").

    /**
     * 新浪微博登录回调
     */
    Get("sinaoauthcallback", func(ctx *goku.HttpContext) goku.ActionResulter {

    sina := utils.NewSaeTOAuth("", "")
    keys := map[string]string{
        "code":         ctx.Get("code"),
        "redirect_uri": "http://www.milnk.com/user/sinaoauthcallback",
    }
    token, err := sina.GetAccessToken("code", keys)
    //fmt.Println("err controler", err)
    //fmt.Println("token", token.Access_Token)
    if len(token.Access_Token) == 0 {
        ctx.ViewData["errorMsg"] = "新浪微博登录异常,请重新登录!"
        return ctx.Render("error", nil)
    }

    weibo := utils.NewSinaWeiBo(token)
    var sinaUser utils.SinaUserInfo
    sinaUser, err = weibo.GetUserInfo()
    if len(sinaUser.Screen_Name) == 0 {
        ctx.ViewData["errorMsg"] = "新浪微博登录异常,请重新登录!"
        return ctx.Render("error", nil)
    }

    var userId int64
    var email string
    userId, email, err = models.Exists_Reference_System_User(token.Access_Token, token.Uid, 1)
    if err != nil {
        ctx.ViewData["errorMsg"] = err.Error()
        return ctx.Render("error", nil)
    }

    if userId > 0 {
        //写cookie
        setCookieForOtherPlatformUser(userId, email, token.Expires_In, ctx)

        return ctx.Redirect("/")
    } else {
        //让用户填补用户名\email
        ctx.ViewData["screenname"] = sinaUser.Screen_Name
        ctx.ViewData["token"] = token.Access_Token
        ctx.ViewData["uid"] = token.Uid
        ctx.ViewData["expires"] = token.Expires_In
    }

    return ctx.Render("oauthcallback", nil)
}).
    /**
     * 新浪微博登录,提交email和昵称
     */
    Post("sinaoauthcallback", func(ctx *goku.HttpContext) goku.ActionResulter {

    email := form.NewEmailField("email", "Email", true).
        Error("invalid", "Email地址错误").
        Error("required", "Email地址必须填写").Field()

    name := form.NewCharField("name", "昵称", true).Min(2).Max(15).
        Error("required", "昵称必须填写").
        Error("range", "昵称长度必须在{0}到{1}之间").Field()

    // add the fields to a form
    f := form.NewForm(email, name)
    f.FillByRequest(ctx.Request)

    errorMsgs := make([]string, 0)
    userId := int64(0)
    if f.Valid() {
        m := f.CleanValues()
        // 检查email地址是否已经注册
        emailExist := models.User_IsEmailExist(m["email"].(string))
        userExist := models.User_IsUserExist(m["name"].(string))
        if !emailExist && !userExist {
            m["reference_id"] = ctx.Get("uid")
            m["reference_system"] = 1
            m["reference_token"] = ctx.Get("token")
            m["create_time"] = time.Now()
            result, err := models.User_SaveMap(m) // TODO:
            if err != nil {
                errorMsgs = append(errorMsgs, golink.ERROR_DATABASE)
                goku.Logger().Errorln(err)
            } else {
                userId, _ = result.LastInsertId()
            }
        } else {
            if userExist {
                errorMsgs = append(errorMsgs, "用户名已经被注册，请换一个")
            }
            if emailExist {
                errorMsgs = append(errorMsgs, "Email地址已经被注册，请换一个")
            }
        }
    } else {
        errs := f.Errors()
        for _, v := range errs {
            errorMsgs = append(errorMsgs, v[0]+": "+v[1])
        }
    }
    // fmt.Println("userId", userId)
    v := f.Values()
    if len(errorMsgs) < 1 {
        //TODO: 过期时间太短了
        seconds, _ := strconv.Atoi(ctx.Get("expires"))
        setCookieForOtherPlatformUser(userId, strings.ToLower(v["email"]), seconds, ctx)

        return ctx.Redirect("/")

    } else {
        ctx.ViewData["regErrors"] = errorMsgs
        ctx.ViewData["screenname"] = v["name"]
        ctx.ViewData["email"] = v["email"]
        ctx.ViewData["token"] = ctx.Get("token")
        ctx.ViewData["uid"] = ctx.Get("uid")
        ctx.ViewData["expires"] = ctx.Get("expires")
    }

    return ctx.Render("oauthcallback", nil)
}).

    /**
     * login view
     */
    Get("login", func(ctx *goku.HttpContext) goku.ActionResulter {

    if u, ok := ctx.Data["user"]; ok && u != nil {
        return ctx.Redirect("/")
    }
    ctx.ViewData["query"] = template.URL(ctx.Request.URL.RawQuery)
    return ctx.View(nil)
}).

    /**
     * reg view
     */
    Get("reg", func(ctx *goku.HttpContext) goku.ActionResulter {

    if u, ok := ctx.Data["user"]; ok && u != nil {
        return ctx.Redirect("/")
    }
    ctx.ViewData["query"] = template.URL(ctx.Request.URL.RawQuery)
    ctx.ViewData["key"] = ctx.Get("key")
    ctx.ViewData["InviteEnabled"] = golink.Invite_Enabled

    return ctx.Render("login", nil)
}).

    /**
     * logout
     */
    Get("logout", func(ctx *goku.HttpContext) goku.ActionResulter {

    redisClient := models.GetRedis()
    defer redisClient.Quit()
    redisClient.Del("_glut")
    c := &http.Cookie{
        Name:    "_glut",
        Expires: time.Now().Add(-10 * time.Second),
        Path:    "/",
    }
    ctx.SetCookie(c)
    return ctx.Redirect("/")
}).

    /**
     * login
     */
    Post("login", func(ctx *goku.HttpContext) goku.ActionResulter {

    f := createLoginForm()
    f.FillByRequest(ctx.Request)

    errorMsgs := make([]string, 0)
    if f.Valid() {
        m := f.CleanValues()
        email := strings.ToLower(m["email"].(string))
        // 检查密码是否正确
        userId := models.User_CheckPwd(email, m["pwd"].(string))
        if userId > 0 {
            now := time.Now()
            h := md5.New()
            h.Write([]byte(fmt.Sprintf("%v-%v", email, now.Unix())))
            ticket := fmt.Sprintf("%x_%v", h.Sum(nil), now.Unix())
            var expires time.Time
            if m["remeber_me"] == "1" {
                expires = now.Add(365 * 24 * time.Hour)
            } else {
                expires = now.Add(48 * time.Hour)
            }
            redisClient := models.GetRedis()
            defer redisClient.Quit()
            err := redisClient.Set(ticket, userId)
            if err != nil {
                goku.Logger().Errorln(err.Error())
                errorMsgs = append(errorMsgs, "登陆票据服务器出错，请重试")
            } else {
                _, err = redisClient.Expireat(ticket, expires.Unix())
                if err != nil {
                    goku.Logger().Errorln(err.Error())
                    errorMsgs = append(errorMsgs, "登陆票据服务器出错，请重试")
                }
                c := &http.Cookie{
                    Name:    "_glut",
                    Value:   ticket,
                    Expires: expires,
                    //Domain:     ".godev.local", // edit (or omit)
                    Path:     "/", // ^ ditto
                    HttpOnly: true,
                }
                ctx.SetCookie(c)
            }
        } else {
            errorMsgs = append(errorMsgs, "账号密码不正确，请改正")
        }
    } else {
        errs := f.Errors()
        for _, v := range errs {
            errorMsgs = append(errorMsgs, v[0]+": "+v[1])
        }
    }

    if len(errorMsgs) < 1 {
        // ctx.ViewData["loginSuccess"] = true
        returnUrl := ctx.Request.FormValue("returnurl")
        if returnUrl == "" {
            returnUrl = "/"
        }
        return ctx.Redirect(returnUrl)
    } else {
        ctx.ViewData["loginErrors"] = errorMsgs
        ctx.ViewData["loginValues"] = f.Values()
    }

    return ctx.Render("login", nil)
}).

    /**
     * submit reg
     */
    Post("reg", func(ctx *goku.HttpContext) goku.ActionResulter {

    f := createRegForm()
    f.FillByRequest(ctx.Request)

    errorMsgs := make([]string, 0)
    if f.Valid() {
        m := f.CleanValues()
        if m["pwd"] == m["repwd"] {

            // 检查email地址是否已经注册
            emailExist := models.User_IsEmailExist(m["email"].(string))
            userExist := models.User_IsUserExist(m["name"].(string))
            regKey := models.VerifyInviteKey(m["key"].(string))
            if !emailExist && !userExist && regKey != nil {
                m["pwd"] = utils.PasswordHash(m["pwd"].(string))
                delete(m, "repwd")
                delete(m, "key")
                m["create_time"] = time.Now()
                _, err := models.User_SaveMap(m)
                if err == nil {
                    models.UpdateIsRegister(regKey)
                } else {
                    errorMsgs = append(errorMsgs, golink.ERROR_DATABASE)
                    goku.Logger().Errorln(err)
                }
            } else {
                if userExist {
                    errorMsgs = append(errorMsgs, "用户名已经被注册，请换一个")
                }
                if emailExist {
                    errorMsgs = append(errorMsgs, "Email地址已经被注册，请换一个")
                }
                if regKey == nil {
                    errorMsgs = append(errorMsgs, "邀请码不正确，可能已经被注册或过期")
                }
            }
        } else {
            errorMsgs = append(errorMsgs, "两次输入的密码不一样")
        }
    } else {
        errs := f.Errors()
        for _, v := range errs {
            errorMsgs = append(errorMsgs, v[0]+": "+v[1])
        }
    }

    if len(errorMsgs) < 1 {
        ctx.ViewData["regSuccess"] = true
    } else {
        ctx.ViewData["regErrors"] = errorMsgs
        ctx.ViewData["regValues"] = f.Values()
        ctx.ViewData["key"] = f.Values()["key"]
    }

    return ctx.Render("login", nil)
})
