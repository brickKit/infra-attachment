from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Parent(_message.Message):
    __slots__ = ("resource_type", "record_id")
    RESOURCE_TYPE_FIELD_NUMBER: _ClassVar[int]
    RECORD_ID_FIELD_NUMBER: _ClassVar[int]
    resource_type: str
    record_id: str
    def __init__(self, resource_type: _Optional[str] = ..., record_id: _Optional[str] = ...) -> None: ...

class CreateUploadRequest(_message.Message):
    __slots__ = ("parent", "category", "file_name", "size", "content_type", "user", "replace_attachment_id")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    FILE_NAME_FIELD_NUMBER: _ClassVar[int]
    SIZE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    USER_FIELD_NUMBER: _ClassVar[int]
    REPLACE_ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    category: str
    file_name: str
    size: int
    content_type: str
    user: str
    replace_attachment_id: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., category: _Optional[str] = ..., file_name: _Optional[str] = ..., size: _Optional[int] = ..., content_type: _Optional[str] = ..., user: _Optional[str] = ..., replace_attachment_id: _Optional[str] = ...) -> None: ...

class CreateUploadResponse(_message.Message):
    __slots__ = ("attachment_id", "version", "upload_url", "confirm_credential", "expires_at")
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    UPLOAD_URL_FIELD_NUMBER: _ClassVar[int]
    CONFIRM_CREDENTIAL_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    attachment_id: str
    version: int
    upload_url: str
    confirm_credential: str
    expires_at: str
    def __init__(self, attachment_id: _Optional[str] = ..., version: _Optional[int] = ..., upload_url: _Optional[str] = ..., confirm_credential: _Optional[str] = ..., expires_at: _Optional[str] = ...) -> None: ...

class IssueDownloadRequest(_message.Message):
    __slots__ = ("parent", "attachment_id", "user")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    USER_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    attachment_id: str
    user: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., attachment_id: _Optional[str] = ..., user: _Optional[str] = ...) -> None: ...

class IssueDownloadResponse(_message.Message):
    __slots__ = ("url", "credential", "expires_at")
    URL_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    url: str
    credential: str
    expires_at: str
    def __init__(self, url: _Optional[str] = ..., credential: _Optional[str] = ..., expires_at: _Optional[str] = ...) -> None: ...

class ListRequest(_message.Message):
    __slots__ = ("parent",)
    PARENT_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ...) -> None: ...

class Attachment(_message.Message):
    __slots__ = ("id", "category", "state", "file_name", "size", "content_type", "version", "unscanned", "created_at", "sensitivity")
    ID_FIELD_NUMBER: _ClassVar[int]
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    FILE_NAME_FIELD_NUMBER: _ClassVar[int]
    SIZE_FIELD_NUMBER: _ClassVar[int]
    CONTENT_TYPE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    UNSCANNED_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    SENSITIVITY_FIELD_NUMBER: _ClassVar[int]
    id: str
    category: str
    state: str
    file_name: str
    size: int
    content_type: str
    version: int
    unscanned: bool
    created_at: str
    sensitivity: str
    def __init__(self, id: _Optional[str] = ..., category: _Optional[str] = ..., state: _Optional[str] = ..., file_name: _Optional[str] = ..., size: _Optional[int] = ..., content_type: _Optional[str] = ..., version: _Optional[int] = ..., unscanned: _Optional[bool] = ..., created_at: _Optional[str] = ..., sensitivity: _Optional[str] = ...) -> None: ...

class ListResponse(_message.Message):
    __slots__ = ("attachments",)
    ATTACHMENTS_FIELD_NUMBER: _ClassVar[int]
    attachments: _containers.RepeatedCompositeFieldContainer[Attachment]
    def __init__(self, attachments: _Optional[_Iterable[_Union[Attachment, _Mapping]]] = ...) -> None: ...

class DetachRequest(_message.Message):
    __slots__ = ("parent", "attachment_id", "retention_start")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    RETENTION_START_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    attachment_id: str
    retention_start: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., attachment_id: _Optional[str] = ..., retention_start: _Optional[str] = ...) -> None: ...

class DetachResponse(_message.Message):
    __slots__ = ("detached",)
    DETACHED_FIELD_NUMBER: _ClassVar[int]
    detached: int
    def __init__(self, detached: _Optional[int] = ...) -> None: ...

class RestoreRequest(_message.Message):
    __slots__ = ("parent", "attachment_id")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    attachment_id: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., attachment_id: _Optional[str] = ...) -> None: ...

class RestoreResponse(_message.Message):
    __slots__ = ("state",)
    STATE_FIELD_NUMBER: _ClassVar[int]
    state: str
    def __init__(self, state: _Optional[str] = ...) -> None: ...

class SetRetentionStartRequest(_message.Message):
    __slots__ = ("parent", "attachment_id", "date")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    DATE_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    attachment_id: str
    date: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., attachment_id: _Optional[str] = ..., date: _Optional[str] = ...) -> None: ...

class SetRetentionStartResponse(_message.Message):
    __slots__ = ("set",)
    SET_FIELD_NUMBER: _ClassVar[int]
    set: int
    def __init__(self, set: _Optional[int] = ...) -> None: ...

class EraseRequest(_message.Message):
    __slots__ = ("parent", "attachment_id", "categories")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    CATEGORIES_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    attachment_id: str
    categories: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., attachment_id: _Optional[str] = ..., categories: _Optional[_Iterable[str]] = ...) -> None: ...

class EraseResponse(_message.Message):
    __slots__ = ("purged", "held")
    PURGED_FIELD_NUMBER: _ClassVar[int]
    HELD_FIELD_NUMBER: _ClassVar[int]
    purged: int
    held: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, purged: _Optional[int] = ..., held: _Optional[_Iterable[str]] = ...) -> None: ...

class HoldRequest(_message.Message):
    __slots__ = ("parent", "attachment_id", "reason")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    ATTACHMENT_ID_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    attachment_id: str
    reason: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., attachment_id: _Optional[str] = ..., reason: _Optional[str] = ...) -> None: ...

class HoldResponse(_message.Message):
    __slots__ = ("hold_id",)
    HOLD_ID_FIELD_NUMBER: _ClassVar[int]
    hold_id: str
    def __init__(self, hold_id: _Optional[str] = ...) -> None: ...

class ReleaseHoldRequest(_message.Message):
    __slots__ = ("parent", "hold_id")
    PARENT_FIELD_NUMBER: _ClassVar[int]
    HOLD_ID_FIELD_NUMBER: _ClassVar[int]
    parent: Parent
    hold_id: str
    def __init__(self, parent: _Optional[_Union[Parent, _Mapping]] = ..., hold_id: _Optional[str] = ...) -> None: ...

class ReleaseHoldResponse(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...
