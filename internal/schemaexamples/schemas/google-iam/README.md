# Google IAM in SpiceDB

Models the permissions of Google's Cloud IAM in SpiceDB. Blog post: <https://authzed.com/blog/google-cloud-iam-modeling/>

---

## Schema

```zed
definition user {}

definition role {
    relation spanner_databaseoperations_cancel: user:*
    relation spanner_databaseoperations_delete: user:*
    relation spanner_databaseoperations_get: user:*
    relation spanner_databaseoperations_list: user:*
    relation spanner_databaseroles_list: user:*
    relation spanner_databaseroles_use: user:*
    relation spanner_databases_beginorrollbackreadwritetransaction: user:*
    relation spanner_databases_beginpartitioneddmltransaction: user:*
    relation spanner_databases_beginreadonlytransaction: user:*
    relation spanner_databases_create: user:*
    relation spanner_databases_drop: user:*
    relation spanner_databases_get: user:*
    relation spanner_databases_getddl: user:*
    relation spanner_databases_getiampolicy: user:*
    relation spanner_databases_list: user:*
    relation spanner_databases_partitionquery: user:*
    relation spanner_databases_partitionread: user:*
    relation spanner_databases_read: user:*
    relation spanner_databases_select: user:*
    relation spanner_databases_setiampolicy: user:*
    relation spanner_databases_update: user:*
    relation spanner_databases_updateddl: user:*
    relation spanner_databases_userolebasedaccess: user:*
    relation spanner_databases_write: user:*
    relation spanner_instances_get: user:*
    relation spanner_instances_getiampolicy: user:*
    relation spanner_instances_list: user:*
    relation spanner_sessions_create: user:*
    relation spanner_sessions_delete: user:*
    relation spanner_sessions_get: user:*
    relation spanner_sessions_list: user:*
}

definition role_binding {
    relation user: user
    relation role: role

    permission spanner_databaseoperations_cancel = user & role->spanner_databaseoperations_cancel
    permission spanner_databaseoperations_delete = user & role->spanner_databaseoperations_delete
    permission spanner_databaseoperations_get = user & role->spanner_databaseoperations_get
    permission spanner_databaseoperations_list = user & role->spanner_databaseoperations_list
    permission spanner_databaseroles_list = user & role->spanner_databaseroles_list
    permission spanner_databaseroles_use = user & role->spanner_databaseroles_use
    permission spanner_databases_beginorrollbackreadwritetransaction = user & role->spanner_databases_beginorrollbackreadwritetransaction
    permission spanner_databases_beginpartitioneddmltransaction = user & role->spanner_databases_beginpartitioneddmltransaction
    permission spanner_databases_beginreadonlytransaction = user & role->spanner_databases_beginreadonlytransaction
    permission spanner_databases_create = user & role->spanner_databases_create
    permission spanner_databases_drop = user & role->spanner_databases_drop
    permission spanner_databases_get = user & role->spanner_databases_get
    permission spanner_databases_getddl = user & role->spanner_databases_getddl
    permission spanner_databases_getiampolicy = user & role->spanner_databases_getiampolicy
    permission spanner_databases_list = user & role->spanner_databases_list
    permission spanner_databases_partitionquery = user & role->spanner_databases_partitionquery
    permission spanner_databases_partitionread = user & role->spanner_databases_partitionread
    permission spanner_databases_read = user & role->spanner_databases_read
    permission spanner_databases_select = user & role->spanner_databases_select
    permission spanner_databases_setiampolicy = user & role->spanner_databases_setiampolicy
    permission spanner_databases_update = user & role->spanner_databases_update
    permission spanner_databases_updateddl = user & role->spanner_databases_updateddl
    permission spanner_databases_userolebasedaccess = user & role->spanner_databases_userolebasedaccess
    permission spanner_databases_write = user & role->spanner_databases_write
    permission spanner_instances_get = user & role->spanner_instances_get
    permission spanner_instances_getiampolicy = user & role->spanner_instances_getiampolicy
    permission spanner_instances_list = user & role->spanner_instances_list
    permission spanner_sessions_create = user & role->spanner_sessions_create
    permission spanner_sessions_delete = user & role->spanner_sessions_delete
    permission spanner_sessions_get = user & role->spanner_sessions_get
    permission spanner_sessions_list = user & role->spanner_sessions_list
}

definition project {
    relation granted: role_binding

    // Synthetic Instance Relations
    permission granted_spanner_instances_get = granted->spanner_instances_get
    permission granted_spanner_instances_getiampolicy = granted->spanner_instances_getiampolicy
    permission granted_spanner_instances_list = granted->spanner_instances_list

    // Synthetic Database Relations
    permission granted_spanner_databases_beginorrollbackreadwritetransaction = granted->spanner_databases_beginorrollbackreadwritetransaction
    permission granted_spanner_databases_beginpartitioneddmltransaction = granted->spanner_databases_beginpartitioneddmltransaction
    permission granted_spanner_databases_beginreadonlytransaction = granted->spanner_databases_beginreadonlytransaction
    permission granted_spanner_databases_create = granted->spanner_databases_create
    permission granted_spanner_databases_drop = granted->spanner_databases_drop
    permission granted_spanner_databases_get = granted->spanner_databases_get
    permission granted_spanner_databases_getddl = granted->spanner_databases_getddl
    permission granted_spanner_databases_getiampolicy = granted->spanner_databases_getiampolicy
    permission granted_spanner_databases_list = granted->spanner_databases_list
    permission granted_spanner_databases_partitionquery = granted->spanner_databases_partitionquery
    permission granted_spanner_databases_partitionread = granted->spanner_databases_partitionread
    permission granted_spanner_databases_read = granted->spanner_databases_read
    permission granted_spanner_databases_select = granted->spanner_databases_select
    permission granted_spanner_databases_setiampolicy = granted->spanner_databases_setiampolicy
    permission granted_spanner_databases_update = granted->spanner_databases_update
    permission granted_spanner_databases_updateddl = granted->spanner_databases_updateddl
    permission granted_spanner_databases_userolebasedaccess = granted->spanner_databases_userolebasedaccess
    permission granted_spanner_databases_write = granted->spanner_databases_write

    // Synthetic Sessions Relations
    permission granted_spanner_sessions_create = granted->spanner_sessions_create
    permission granted_spanner_sessions_delete = granted->spanner_sessions_delete
    permission granted_spanner_sessions_get = granted->spanner_sessions_get
    permission granted_spanner_sessions_list = granted->spanner_sessions_list

    // Synthetic Database Operations Relations
    permission granted_spanner_databaseoperations_cancel = granted->spanner_databaseoperations_cancel
    permission granted_spanner_databaseoperations_delete = granted->spanner_databaseoperations_delete
    permission granted_spanner_databaseoperations_get = granted->spanner_databaseoperations_get
    permission granted_spanner_databaseoperations_list = granted->spanner_databaseoperations_list

    // Synthetic Database Roles Relations
    permission granted_spanner_databaseroles_list = granted->spanner_databaseroles_list
    permission granted_spanner_databaseroles_use = granted->spanner_databaseroles_use
}

definition spanner_instance {
    relation project: project
    relation granted: role_binding

    permission get = granted->spanner_instances_get + project->granted_spanner_instances_get
    permission getiampolicy = granted->spanner_instances_getiampolicy + project->granted_spanner_instances_getiampolicy
    permission list = granted->spanner_instances_list + project->granted_spanner_instances_list

    // Synthetic Database Relations
    permission granted_spanner_databases_beginorrollbackreadwritetransaction = granted->spanner_databases_beginorrollbackreadwritetransaction + project->granted_spanner_databases_beginorrollbackreadwritetransaction
    permission granted_spanner_databases_beginpartitioneddmltransaction = granted->spanner_databases_beginpartitioneddmltransaction + project->granted_spanner_databases_beginpartitioneddmltransaction
    permission granted_spanner_databases_beginreadonlytransaction = granted->spanner_databases_beginreadonlytransaction + project->granted_spanner_databases_beginreadonlytransaction
    permission granted_spanner_databases_create = granted->spanner_databases_create + project->granted_spanner_databases_create
    permission granted_spanner_databases_drop = granted->spanner_databases_drop + project->granted_spanner_databases_drop
    permission granted_spanner_databases_get = granted->spanner_databases_get + project->granted_spanner_databases_get
    permission granted_spanner_databases_getddl = granted->spanner_databases_getddl + project->granted_spanner_databases_getddl
    permission granted_spanner_databases_getiampolicy = granted->spanner_databases_getiampolicy + project->granted_spanner_databases_getiampolicy
    permission granted_spanner_databases_list = granted->spanner_databases_list + project->granted_spanner_databases_list
    permission granted_spanner_databases_partitionquery = granted->spanner_databases_partitionquery + project->granted_spanner_databases_partitionquery
    permission granted_spanner_databases_partitionread = granted->spanner_databases_partitionread + project->granted_spanner_databases_partitionread
    permission granted_spanner_databases_read = granted->spanner_databases_read + project->granted_spanner_databases_read
    permission granted_spanner_databases_select = granted->spanner_databases_select + project->granted_spanner_databases_select
    permission granted_spanner_databases_setiampolicy = granted->spanner_databases_setiampolicy + project->granted_spanner_databases_setiampolicy
    permission granted_spanner_databases_update = granted->spanner_databases_update + project->granted_spanner_databases_update
    permission granted_spanner_databases_updateddl = granted->spanner_databases_updateddl + project->granted_spanner_databases_updateddl
    permission granted_spanner_databases_userolebasedaccess = granted->spanner_databases_userolebasedaccess + project->granted_spanner_databases_userolebasedaccess
    permission granted_spanner_databases_write = granted->spanner_databases_write + project->granted_spanner_databases_write

    // Synthetic Sessions Relations
    permission granted_spanner_sessions_create = granted->spanner_sessions_create + project->granted_spanner_sessions_create
    permission granted_spanner_sessions_delete = granted->spanner_sessions_delete + project->granted_spanner_sessions_delete
    permission granted_spanner_sessions_get = granted->spanner_sessions_get + project->granted_spanner_sessions_get
    permission granted_spanner_sessions_list = granted->spanner_sessions_list + project->granted_spanner_sessions_list

    // Synthetic Database Operations Relations
    permission granted_spanner_databaseoperations_cancel = granted->spanner_databaseoperations_cancel + project->granted_spanner_databaseoperations_cancel
    permission granted_spanner_databaseoperations_delete = granted->spanner_databaseoperations_delete + project->granted_spanner_databaseoperations_delete
    permission granted_spanner_databaseoperations_get = granted->spanner_databaseoperations_get + project->granted_spanner_databaseoperations_get
    permission granted_spanner_databaseoperations_list = granted->spanner_databaseoperations_list + project->granted_spanner_databaseoperations_list

    // Synthetic Database Roles Relations
    permission granted_spanner_databaseroles_list = granted->spanner_databaseroles_list + project->granted_spanner_databaseroles_list
    permission granted_spanner_databaseroles_use = granted->spanner_databaseroles_use + project->granted_spanner_databaseroles_use
}

definition spanner_database {
    relation instance: spanner_instance
    relation granted: role_binding

    // Database
    permission beginorrollbackreadwritetransaction = granted->spanner_databases_beginorrollbackreadwritetransaction + instance->granted_spanner_databases_beginorrollbackreadwritetransaction
    permission beginpartitioneddmltransaction = granted->spanner_databases_beginpartitioneddmltransaction + instance->granted_spanner_databases_beginpartitioneddmltransaction
    permission beginreadonlytransaction = granted->spanner_databases_beginreadonlytransaction + instance->granted_spanner_databases_beginreadonlytransaction
    permission create = granted->spanner_databases_create + instance->granted_spanner_databases_create
    permission drop = granted->spanner_databases_drop + instance->granted_spanner_databases_drop
    permission get = granted->spanner_databases_get + instance->granted_spanner_databases_get
    permission get_ddl = granted->spanner_databases_getddl + instance->granted_spanner_databases_getddl
    permission getiampolicy = granted->spanner_databases_getiampolicy + instance->granted_spanner_databases_getiampolicy
    permission list = granted->spanner_databases_list + instance->granted_spanner_databases_list
    permission partitionquery = granted->spanner_databases_partitionquery + instance->granted_spanner_databases_partitionquery
    permission partitionread = granted->spanner_databases_partitionread + instance->granted_spanner_databases_partitionread
    permission read = granted->spanner_databases_read + instance->granted_spanner_databases_read
    permission select = granted->spanner_databases_select + instance->granted_spanner_databases_select
    permission setiampolicy = granted->spanner_databases_setiampolicy + instance->granted_spanner_databases_setiampolicy
    permission update = granted->spanner_databases_update + instance->granted_spanner_databases_update
    permission updateddl = granted->spanner_databases_updateddl + instance->granted_spanner_databases_updateddl
    permission userolebasedaccess = granted->spanner_databases_userolebasedaccess + instance->granted_spanner_databases_userolebasedaccess
    permission write = granted->spanner_databases_write + instance->granted_spanner_databases_write

    // Sessions
    permission create_session = granted->spanner_sessions_create + instance->granted_spanner_sessions_create
    permission delete_session = granted->spanner_sessions_delete + instance->granted_spanner_sessions_delete
    permission get_session = granted->spanner_sessions_get + instance->granted_spanner_sessions_get
    permission list_sessions = granted->spanner_sessions_list + instance->granted_spanner_sessions_list

    // Database Operations
    permission cancel_operation = granted->spanner_databaseoperations_cancel + instance->granted_spanner_databaseoperations_cancel
    permission delete_operation = granted->spanner_databaseoperations_delete + instance->granted_spanner_databaseoperations_delete
    permission get_operation = granted->spanner_databaseoperations_get + instance->granted_spanner_databaseoperations_get
    permission list_operations = granted->spanner_databaseoperations_list + instance->granted_spanner_databaseoperations_list

    // Database Roles
    permission list_roles = granted->spanner_databaseroles_list + instance->granted_spanner_databaseroles_list
    permission use_role = granted->spanner_databaseroles_use + instance->granted_spanner_databaseroles_use
}
```
