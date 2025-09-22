# User Defined Roles

Models user defined custom roles. Blog post: <https://authzed.com/blog/user-defined-roles/>

---

## Schema

```zed
definition user {}

definition project {
    relation issue_creator: role#member
    relation issue_assigner: role#member
    relation any_issue_resolver: role#member
    relation assigned_issue_resolver: role#member
    relation comment_creator: role#member
    relation comment_deleter: role#member
    relation role_manager: role#member

    permission create_issue = issue_creator
    permission create_role = role_manager
}

definition role {
    relation project: project
    relation member: user
    relation built_in_role: project

    permission delete = project->role_manager - built_in_role->role_manager
    permission add_user = project->role_manager
    permission add_permission = project->role_manager - built_in_role->role_manager
    permission remove_permission = project->role_manager - built_in_role->role_manager
}

definition issue {
    relation project: project
    relation assigned: user

    permission assign = project->issue_assigner
    permission resolve = (project->assigned_issue_resolver & assigned) + project->any_issue_resolver
    permission create_comment = project->comment_creator

    // synthetic relation
    permission project_comment_deleter = project->comment_deleter
}

definition comment {
    relation issue: issue
    permission delete = issue->project_comment_deleter
}
```
