create table patches (
  id integer primary key autoincrement,
  name varchar(50) default null unique,
  title varchar(256) default null,
  description varchar(256) default null,
  created_at timestamp default (datetime('now','localtime'))
);
