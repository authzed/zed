# Caveats for conditional access

Models the use of caveats, which allows for conditional access based on information provided at _runtime_ to permission checks.

---

## Schema

```zed
definition user {}

/**
 * only allowed on tuesdays. `day_of_week` can be provided either at the time
 * the relationship is written, or in the CheckPermission API call.
 */
caveat only_on_tuesday(day_of_week string) {
  day_of_week == 'tuesday'
}

definition document {
    /**
     * reader indicates that the user is a reader on the document, either
     * directly or only on tuesday.
     */
    relation reader: user | user with only_on_tuesday

    permission view = reader
}
```
