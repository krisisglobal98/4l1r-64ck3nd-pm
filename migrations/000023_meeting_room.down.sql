DELETE FROM pm.code_prefix WHERE lower(code) IN ('building', 'room_amenity', 'meeting_room');
DROP TABLE pm.meeting_room_amenity;
DROP TABLE pm.meeting_room;
DROP TABLE pm.room_amenity;
DROP TABLE pm.building;
