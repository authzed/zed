# Simple Role Based Access Control

Models Role Based Access Control (RBAC), where access is granted to users based on the role(s) in which they are a member.

---

## Schema

```zed
/**
 * user represents a user that can be granted role(s)
 */
definition user {}

/**
 * document represents a document protected by Authzed.
 */
definition document {
    /**
     * writer indicates that the user is a writer on the document.
     */
    relation writer: user

    /**
     * reader indicates that the user is a reader on the document.
     */
    relation reader: user

    /**
     * edit indicates that the user has permission to edit the document.
     */
    permission edit = writer

    /**
     * view indicates that the user has permission to view the document, if they
     * are a `reader` *or* have `edit` permission.
     */
    permission view = reader + edit
}
```

The RBAC example defines two kinds of objects: `user` to be used as references to users and `document`, representing a resource being protected (in this case, a document).

### user

`user` is an example of a "user" definition, which is used to represent users. The definition itself is empty, as it is only used for referencing purposes.

```zed
definition user {}
```

### document

`document` is an example of a "resource" definition, which is used to define the relations and permissions for a specific kind of resource. Here, that resource is a document.

```zed
definition document {
    /**
     * writer indicates that the user is a writer on the document.
     */
    relation writer: user

    /**
     * reader indicates that the user is a reader on the document.
     */
    relation reader: user

    /**
     * edit indicates that the user has permission to edit the document.
     */
    permission edit = writer

    /**
     * view indicates that the user has permission to view the document, if they
     * are a `reader` *or* have `edit` permission.
     */
    permission view = reader + edit
}
```

Within the `document` definition, there are defined two relations `reader` and `writer`, which are used to represent roles for users, and two permissions `edit` and `view`, which represent the permissions that can be checked on a document.

#### writer

The `writer` relation defines a role of "writer" for users.

#### reader

The `reader` relation defines a role of "reader" for users.

#### edit

The `edit` permission defines an edit permission on the document.

#### view

The `view` permission defines a view permission on the document.

Note that `view` includes the `edit` permission. This means that if a user is granted the role of `writer` and thus, has `edit` permission, they will _also_ be implicitly granted the permission of `view`.
