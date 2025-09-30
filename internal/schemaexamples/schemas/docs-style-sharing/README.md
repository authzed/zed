# Google Docs-style Sharing

Models a Google Docs-style sharing permission system where users can be granted direct access to a resource, or access via organizations and nested groups.

---

## Schema

```zed
definition user {}

definition resource {
    relation manager: user | usergroup#member | usergroup#manager
    relation viewer: user | usergroup#member | usergroup#manager

    permission manage = manager
    permission view = viewer + manager
}

definition usergroup {
    relation manager: user | usergroup#member | usergroup#manager
    relation direct_member: user | usergroup#member | usergroup#manager

    permission member = direct_member + manager
}

definition organization {
    relation group: usergroup
    relation administrator: user | usergroup#member | usergroup#manager
    relation direct_member: user

    relation resource: resource

    permission admin = administrator
    permission member = direct_member + administrator + group->member
}
```

### user

`user` is an example of a "user" type, which is used to represent users. The definition itself is empty, as it is only used for referencing purposes.

```zed
definition user {}
```

### resource

`resource` is the definition used to represent the resource being shared

```zed
definition resource {
    relation manager: user | usergroup#member | usergroup#manager
    relation viewer: user | usergroup#member | usergroup#manager

    permission manage = manager
    permission view = viewer + manager
}
```

Within the definition, there are defined two relations: `viewer` and `manager`, which are used to represent roles for users _or members/managers of groups_ for the resource, as well as the `view` and `manage` permissions for viewing and managing the resource, respectively.

### usergroup

`usergroup` is the definition used to represent groups, which can contain either users or other groups. Groups support a distinction between member and manager.

```zed
definition usergroup {
    relation manager: user | usergroup#member | usergroup#manager
    relation direct_member: user | usergroup#member | usergroup#manager

    permission member = direct_member + manager
}
```

### organization

`organization` is the definition used to represent the overall organization.

```zed
definition organization {
    relation group: usergroup
    relation administrator: user | usergroup#member | usergroup#manager
    relation direct_member: user

    relation resource: resource

    permission admin = administrator
    permission member = direct_member + administrator + group->member
}
```

Organizations contain four relations (`group`, `resource`, `member`, `administrator`) which are used to reference the groups, resources, direct members and administrator users for the organization.

#### member permission

The `member` permission under organization computes the transitive closure of _all_ member users/groups of an organization by combining data from three sources:

1. `direct_member`: users directly added to the organization as a member
2. `administrator` is used to add any users found as an `administrator` of the organization
3. `group->member` is used to walk from the organization to any of its groups, and then from the `group` to any of its members. This ensure that if a user is available under `member` under any group in the organization, they are treated as a member of the organization as well.
