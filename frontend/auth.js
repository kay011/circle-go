(function (global) {
    var STORAGE_TOKEN = 'circle_token';
    var STORAGE_USER = 'circle_username';

    global.Auth = {
        getToken: function () {
            return localStorage.getItem(STORAGE_TOKEN);
        },
        setAuth: function (token, username) {
            localStorage.setItem(STORAGE_TOKEN, token);
            localStorage.setItem(STORAGE_USER, username || '');
        },
        clearAuth: function () {
            localStorage.removeItem(STORAGE_TOKEN);
            localStorage.removeItem(STORAGE_USER);
        },
        getSavedUsername: function () {
            return localStorage.getItem(STORAGE_USER) || '';
        },
        redirectToLogin: function () {
            window.location.href = 'login.html';
        },
        redirectToRegister: function () {
            window.location.href = 'register.html';
        },
        redirectToApp: function () {
            window.location.href = 'index.html';
        },
    };
})(typeof window !== 'undefined' ? window : this);
