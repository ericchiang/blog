+++
title = "Writing shared libraries using Rust"
date = "2024-02-26"
published = "2024-02-25"
+++

Every tool that gets big enough eventually provides a way to support third-party logic. Maybe you expose APIs for clients to call. Maybe you take some code and run it in a sandbox. Maybe you embed a Lua interpreter.

For many programs, extensibility means dynamic shared libraries. Good old, "here's a .so file for you to dlopen()." [PKCS #11](https://docs.oasis-open.org/pkcs11/pkcs11-base/v2.40/os/pkcs11-base-v2.40-os.html), [Sudo Plugins](https://www.sudo.ws/about/plugins/), [Python](https://docs.python.org/3/extending/extending.html) and [NodeJS](https://nodejs.org/api/addons.html) addons, [SQLite](https://www.sqlite.org/loadext.html) and [Postgres](https://www.postgresql.org/download/products/6-postgresql-extensions/) extensions, [Nginx](https://www.nginx.com/resources/wiki/modules/) and [httpd](https://httpd.apache.org/modules/) modules, even [LD_PRELOAD](https://blog.jessfraz.com/post/ld_preload/) hacks.

As much fun as C is, I've been motivated by [recent](https://github.com/google/native-pkcs11) [projects](https://github.com/square/sudo_pair) that take advantage of Rust's FFI support. This post covers a proof-of-concept Linux-PAM module for [_Google Authenticator_](https://en.wikipedia.org/wiki/Google_Authenticator)*, going through the steps to build a shared library with Rust.

<sub>_*Sure there's an actual module [maintained under Google's GitHub org](https://github.com/google/google-authenticator-libpam), but where's the fun in that?_</sub>

## Pluggable Authentication Modules 

Linux-PAM is a system that agressively embraces shared libraries. Even by default, PAM logic is implemented as shared objects rather than directly in the core library. For example, the default PAM configuration on my Debian machine is setup to call [`pam_unix.so`](https://github.com/linux-pam/linux-pam/blob/v1.6.0/modules/pam_unix), which does the heavy lifting.

```
$ ls /lib/x86_64-linux-gnu/security/
pam_access.so     pam_keyinit.so    pam_pwhistory.so   pam_timestamp.so
pam_debug.so      pam_lastlog.so    pam_rhosts.so      pam_tty_audit.so
pam_deny.so       pam_limits.so     pam_rootok.so      pam_umask.so
pam_echo.so       pam_listfile.so   pam_securetty.so   pam_unix.so
pam_env.so        pam_localuser.so  pam_selinux.so     pam_userdb.so
pam_exec.so       pam_loginuid.so   pam_sepermit.so    pam_usertype.so
pam_faildelay.so  pam_mail.so       pam_setquota.so    pam_warn.so
pam_faillock.so   pam_mkhomedir.so  pam_shells.so      pam_wheel.so
pam_filter.so     pam_motd.so       pam_stress.so      pam_xauth.so
pam_ftp.so        pam_namespace.so  pam_succeed_if.so
pam_group.so      pam_nologin.so    pam_systemd.so
pam_issue.so      pam_permit.so     pam_time.so
$ grep '^auth' /etc/pam.d/common-auth
auth    [success=1 default=ignore]      pam_unix.so nullok
auth    requisite                       pam_deny.so
auth    required                        pam_permit.so
```

Linux-PAM authentication services are expected to export the following API.

```
int pam_sm_authenticate(pam_handle_t *pamh, int flags, int argc,
                        const char **argv);
int pam_sm_setcred(pam_handle_t *pamh, int flags, int argc,
                   const char **argv);
```

`pam_handle_t` is the important type here, providing a handle to query PAM for the user to authenticate ([`pam_get_user(3)`](https://man7.org/linux/man-pages/man3/pam_get_user.3.html)), as well as the entered password ([`pam_get_authtok(3)`](https://man7.org/linux/man-pages/man3/pam_get_authtok.3.html)).

## C and Rust

First things first, create a Rust library.

```
cargo new --lib pam_totp
```

The package's configuration will define a few dependencies to compute TOTP codes, tools for C bindings, and declare it as a C shared library.

```
[package]
name = "pam_totp"
version = "0.1.0"
edition = "2021"

[dependencies]
data-encoding = "2.5.0" # Base32 encoding
ring = "0.17.8"         # HMAC for TOTP logic

[build-dependencies]
bindgen = "0.69.4" # C to Rust bindings generation

[lib]
crate-type = ["lib", "cdylib"]
```

[Bindgen](https://rust-lang.github.io/rust-bindgen/) is Rust's stratey for generating Rust equivalent structs and functions from C. Given a C header file, it produces equivalent Rust representations for the local crate to use.

To tell bindgen what to generate, define a header file including the PAM headers the project needs.

```
// wrapper.h 
#include <security/pam_appl.h>
#include <security/pam_modules.h>
#include <security/pam_ext.h>
```

Bindgen is called through [`build.rs`](https://doc.rust-lang.org/cargo/reference/build-scripts.html), which reads the header file and runs the generator. Since the module will need to use some PAM functions, not just export symbols, it also needs to link against the PAM libraries to pull in implementations.

```
// build.rs
use bindgen;

use std::env;
use std::path::PathBuf;

fn main() {
    println!("cargo:rustc-link-lib=dylib=pam"); // Link against PAM shared libraries.
    let bindings = bindgen::Builder::default()
        .header("wrapper.h")
        // Generate constants that match c_int as i32.
        // See: https://users.rust-lang.org/t/interfacing-c-code-with-bindgen-define-and-types/67595
        .default_macro_constant_type(bindgen::MacroTypeVariation::Signed)
        .parse_callbacks(Box::new(bindgen::CargoCallbacks::new()))
        .generate()
        .expect("Unable to generate bindings");
    let out_path = PathBuf::from(env::var("OUT_DIR").unwrap());
    bindings
        .write_to_file(out_path.join("bindings.rs"))
        .expect("Couldn't write bindings!");
}
```

On Debian this required clang and PAM development headers.

```
sudo apt-get install -y clang libpam0g-dev
```

After building, the generated Rust code will be in the project's debug directory. 

```
$ cargo build
...
$ sed -n "73,92p" target/debug/build/pam_totp-*/out/bindings.rs
#[repr(C)]
#[derive(Debug, Copy, Clone)]
pub struct pam_handle {
    _unused: [u8; 0],
}
pub type pam_handle_t = pam_handle;
extern "C" {
    pub fn pam_set_item(
        pamh: *mut pam_handle_t,
        item_type: ::std::os::raw::c_int,
        item: *const ::std::os::raw::c_void,
    ) -> ::std::os::raw::c_int;
}
extern "C" {
    pub fn pam_get_item(
        pamh: *const pam_handle_t,
        item_type: ::std::os::raw::c_int,
        item: *mut *const ::std::os::raw::c_void,
    ) -> ::std::os::raw::c_int;
}
```

Generated bindings heavily leverage Rust's [pointer type](https://doc.rust-lang.org/std/primitive.pointer.html), a rare lanugage feature for "normal" Rust where references (`&T`) are used instead. Rust pointers behave almost identically to C, but have mutability annotations (`T* t` becomes `t: *mut T`), and require unsafe blocks for almost all operations.

As an example, the C method [`pam_open_session(3)`](https://man7.org/linux/man-pages/man3/pam_open_session.3.html).

```
int pam_open_session(pam_handle_t *pamh, int flags);
```

Is translated to the following.

```
fn pam_open_session(pamh: *mut pam_handle_t, flags: c_int) -> c_int;
```

Finally, to use the bindings, include the generated code in a local module and suppress a ton of lint checks that will complain about C-style naming.

```
// lib/pam.rs
#![allow(non_upper_case_globals)]
#![allow(non_camel_case_types)]
#![allow(non_snake_case)]
#![allow(dead_code)]
include!(concat!(env!("OUT_DIR"), "/bindings.rs"));
```

## Google Authenticator logic

On the Rust side of things, we'll need to prompt the user for a TOTP code and validate it. The library's preamble will import various dependencies and define an error type.

```
// src/lib.rs
mod pam; // Import generated bindings.

// Various other imports.
use data_encoding;
use ring::hmac;
use std::ffi::*;
use std::fmt;
use std::fs;
use std::ptr;
use std::time;

/// Result type used by this application.
///
/// See: https://doc.rust-lang.org/rust-by-example/error/multiple_error_types/define_error_type.html
type Result<T> = std::result::Result<T, Error>;

/// Error type that wraps a string error description.
///
/// See: https://rust-cli.github.io/book/tutorial/errors.html
#[derive(Debug)]
struct Error(String);

impl fmt::Display for Error {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        return write!(f, "{}", self.0);
    }
}

/// Convenience format wrapper that returns an Error type.
macro_rules! errorf {
    ($($arg:tt)*) => {
        Error(format!($($arg)*))
    };
}
```

The TOTP code is a bit tricky, but gets to leverage [github.com/briansmith/ring](https://github.com/briansmith/ring) for the HMAC logic. Ring is a terrific crate for these kinds of lower level crypto primatives.

```
/// Produce a TOTP code using the given key using the same parameters as Google
/// Google Authenticator (period of 30s and 6 digits).
///
/// See:
/// - https://datatracker.ietf.org/doc/html/rfc6238 (TOTP)
/// - https://datatracker.ietf.org/doc/html/rfc4226 (HOTP)
fn totp(k: &[u8]) -> String {
    let now = time::SystemTime::now();
    let secs = now
        .duration_since(time::SystemTime::UNIX_EPOCH)
        .unwrap_or(time::Duration::new(0, 0))
        .as_secs();

    let c = secs / 30;
    let counter: [u8; 8] = [
        (c >> 56) as u8,
        (c >> 48) as u8,
        (c >> 40) as u8,
        (c >> 32) as u8,
        (c >> 24) as u8,
        (c >> 16) as u8,
        (c >> 8) as u8,
        c as u8,
    ];

    let key = hmac::Key::new(hmac::HMAC_SHA1_FOR_LEGACY_USE_ONLY, k);
    let tag = hmac::sign(&key, &counter[..]);

    let a = tag.as_ref();
    let o = (a[a.len() - 1] & 0xf) as usize;
    let n: u32 = ((a[o] & 0x7f) as u32) << 24
        | (a[o + 1] as u32) << 16
        | (a[o + 2] as u32) << 8
        | a[o + 3] as u32;

    return format!("{:06}", n % 1000000);
}
```

The module then needs to access the TOTP secret and compare a generated TOTP code against the user's input.

This is the first point where the library that has to reference PAM methods. Specifically, `pam_get_user` and `pam_get_authtok`. Calling C methods and working with pointers requires `unsafe` blocks sprinkled throughout the code.

```
// Storing a TOTP secret on disk probably isn't a good idea. But the demo gods
// must have their sacrifices.
const SECRET_FILE: &str = "/etc/pam_totp_key";

/// Core pam_sm_authenticate logic, returning a PAM status. The method
/// challenges the user to enter a Google Authenticator code matched against a
/// private key stored in /etc/pam_totp_key.
fn authenticate(pamh: *mut pam::pam_handle_t) -> Result<c_int> {
    // Google Authenticator keys are base32 encoded.
    let totp_key = fs::read_to_string(SECRET_FILE)
        .map_err(|e| errorf!("failed to read totp secret {}: {}", SECRET_FILE, e))?;
    let key = data_encoding::BASE32_NOPAD
        .decode(totp_key.trim().as_bytes())
        .map_err(|e| errorf!("failed to decode totp secret: {}", e))?;

    // Determine the active user. Returned string is owned by PAM and shouldn't
    // be free'd.
    //
    // https://man7.org/linux/man-pages/man3/pam_get_user.3.html
    let mut user_ptr: *const c_char = ptr::null();
    let rc = unsafe { pam::pam_get_user(pamh, &mut user_ptr, ptr::null()) };
    if rc != pam::PAM_SUCCESS {
        return Err(errorf!("failed getting pam user: {}", rc));
    }
    let username = (unsafe { CStr::from_ptr(user_ptr) })
        .to_str()
        .map_err(|e| errorf!("invalid token string: {}", e))?;
    if username != "root" {
        // Skip authenticating users that aren't root.
        return Ok(pam::PAM_AUTH_ERR);
    }

    // Ask the user to enter a TOTP code.
    //
    // https://man7.org/linux/man-pages/man3/pam_get_authtok.3.html
    let mut token_ptr: *const c_char = ptr::null();
    let prompt = CString::new("Enter Google Authenticator code: ")
        .map_err(|e| errorf!("initializing C string: {}", e))?;
    let rc = unsafe {
        pam::pam_get_authtok(
            pamh,
            pam::PAM_AUTHTOK,
            &mut token_ptr,
            prompt.as_c_str().as_ptr(),
        )
    };
    if rc != pam::PAM_SUCCESS {
        return Err(errorf!("failed getting pam token: {}", rc));
    }
    let token = (unsafe { CStr::from_ptr(token_ptr) })
        .to_str()
        .map_err(|e| errorf!("invalid token string: {}", e))?;

    // Validate TOTP code.
    let want = totp(key.as_slice());
    if token.trim() != want {
        return Err(errorf!("invalid totp token"));
    }
    eprintln!("Google Authenticator code matched");
    return Ok(pam::PAM_SUCCESS);
}
```

## Exporting C APIs

Rust has direct support for defining a [C foreign interface](https://doc.rust-lang.org/nomicon/ffi.html). To match the `pam_sm_authenticate` signature expected by Linux-PAM, use the C to Rust translation strategies discussed above to match the argument types, and mark the function `#[no_mangle] pub extern "C"`.

The exported method will wrap the auth logic defined above. `pam_sm_setcred` will be no-op.

```
#[no_mangle]
pub extern "C" fn pam_sm_authenticate(
    pamh: *mut pam::pam_handle_t,
    _flags: c_int,
    _argc: c_int,
    _argv: *const *const c_char,
) -> c_int {
    return match authenticate(pamh) {
        Ok(rc) => rc,
        Err(err) => {
            eprintln!("pam plugin: auth failed: {}", err);
            return pam::PAM_AUTH_ERR;
        }
    };
}

#[no_mangle]
pub extern "C" fn pam_sm_setcred(
    _pamh: *mut pam::pam_handle_t,
    _flags: c_int,
    _argc: c_int,
    _argv: *const *const c_char,
) -> c_int {
    return pam::PAM_SUCCESS;
}
```

As expected, Rust builds a shared library with the PAM symbols.

```
$ cargo build --release
$ nm target/release/libpam_totp.so | grep pam_sm
00000000000087e0 T pam_sm_authenticate
0000000000009060 T pam_sm_setcred
```

To install, copy the shared library to the Linux-PAM modules path.

```
if [ -f "/lib/x86_64-linux-gnu/security/pam_totp.so" ]; then
    # Unlink in case a program is actively holding this open. Fixes issues like
    # sudo segfaulting when the file is overwritten.
    sudo unlink /lib/x86_64-linux-gnu/security/pam_totp.so
fi
sudo cp target/release/libpam_totp.so /lib/x86_64-linux-gnu/security/pam_totp.so
```

And write the base32'd TOTP secret to `/etc` (don't actually do this in a real system).

```
// Is this an HSM?
echo -n "${TOTP_SECRET_KEY}" | sudo tee /etc/pam_totp_key > /dev/null
sudo chmod 0600 /etc/pam_totp_key
```

After adding the shared library as a entry to PAM's auth configuration...

```
# /etc/pam.d/common-auth

# Install the custom TOTP shared library.
auth    sufficient                      pam_totp.so

# Existing configuration.
auth    [success=1 default=ignore]      pam_unix.so nullok
auth    requisite                       pam_deny.so
auth    required                        pam_permit.so
```

`su` will now prompt me for a TOTP code when changing to the "root" user.

```
ericchiang@localhost:~$ su -
Enter Google Authenticator code: 
Google Authenticator code matched
root@localhost:~$
```

Or reject me if the token is wrong.

```
ericchiang@localhost:~$ su -
Enter Google Authenticator code: 
pam plugin: auth failed: invalid totp token
su: Authentication failure
ericchiang@localhost:~$
```

Success!

## Why Rust?

Most of the C code I've written professionally has been for these kinds of plugins. While it's a fun exercise to manually track allocations and "know you've still got it," trying to reason about someone else's memory management during a code review can be rough. The least favorite project I've worked on involed refactoring a C ASN.1 parser.

Rust ends up being a terrific, modern language for system programming. In situations like PAM where I'd previously be inclined to write a C shim to exec something else, Rust provides a robust way to integrate and write custom logic. Be it OS tooling, drivers, or just a PAM extension. 
